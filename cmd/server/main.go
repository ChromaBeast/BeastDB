package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/replication"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
	"google.golang.org/grpc"
)

const Version = "0.1.0-release"

func main() {
	role := flag.String("role", getEnv("BEASTDB_ROLE", "leader"), "Node cluster role: leader or follower")
	bindAddr := flag.String("bind-addr", getEnv("BEASTDB_BIND_ADDR", "127.0.0.1"), "Bind address for gRPC service")
	port := flag.Int("port", getEnvInt("BEASTDB_PORT", 50051), "Port for gRPC service")
	leaderAddr := flag.String("leader-addr", getEnv("BEASTDB_LEADER_ADDR", "127.0.0.1:50051"), "Leader node address for replication")
	dataDir := flag.String("data-dir", getEnv("BEASTDB_DATA_DIR", "./data"), "Directory to store data and WAL files")
	poolSize := flag.Int("pool-size", getEnvInt("BEASTDB_POOL_SIZE", 128), "Buffer pool frame capacity (4KB blocks)")
	replicaID := flag.String("replica-id", getEnv("BEASTDB_REPLICA_ID", "replica-1"), "Unique identifier for this replica node")
	webAddr := flag.String("web-addr", getEnv("BEASTDB_WEB_ADDR", "127.0.0.1:8080"), "Address for the web admin console (empty to disable)")
	adminPassword := flag.String("admin-password", getEnv("BEASTDB_ADMIN_PASSWORD", "admin"), "Initial admin user password (or BEASTDB_ADMIN_PASSWORD)")
	partitionConfig := flag.String("partition-config", getEnv("BEASTDB_PARTITION_CONFIG", ""), "Path to partitions.json defining Studio partition labels (optional)")
	apiToken := flag.String("api-token", getEnv("BEASTDB_API_TOKEN", ""), "Bearer token for gRPC authentication (or BEASTDB_API_TOKEN)")
	tlsCert := flag.String("tls-cert", getEnv("BEASTDB_TLS_CERT", ""), "Path to TLS server/client certificate file (PEM)")
	tlsKey := flag.String("tls-key", getEnv("BEASTDB_TLS_KEY", ""), "Path to TLS server/client private key file (PEM)")
	tlsCA := flag.String("tls-ca", getEnv("BEASTDB_TLS_CA", ""), "Path to TLS CA certificate file (PEM)")
	insecureAuth := flag.Bool("insecure-auth", false, "Permit default credentials on public interfaces (unsafe, for testing only)")
	devMode := flag.Bool("dev", false, "Run in ephemeral emulator mode with temporary storage auto-purged on exit")
	flag.Parse()

	grpcBind := fmt.Sprintf("%s:%d", *bindAddr, *port)
	if *devMode {
		cleanup := setupDevMode(dataDir, webAddr, adminPassword, *port)
		defer cleanup()
	} else {
		if err := validateSecurityConfig(*adminPassword, *webAddr, grpcBind, *devMode, *insecureAuth); err != nil {
			log.Fatalf("FATAL: %v", err)
		}
		if *adminPassword == "admin" {
			log.Println("⚠️  SECURITY WARNING: Using default password 'admin' on loopback. Set BEASTDB_ADMIN_PASSWORD in production!")
		}
	}

	log.Printf("Starting BeastDB v%s [Role: %s] on %s...", Version, *role, grpcBind)
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	engine, err := api.NewEngine(filepath.Join(*dataDir, "beast.bin"), filepath.Join(*dataDir, "beast.wal"), *poolSize)
	if err != nil {
		log.Fatalf("Failed to initialize database engine: %v", err)
	}
	if *role != "leader" {
		engine.SetReadOnly(true)
		log.Printf("🔒 Node running in follower mode: engine write path set to read-only.")
	}

	tokenStore := initTokenStore(engine)
	var grpcOpts []grpc.ServerOption
	if *tlsCert != "" && *tlsKey != "" {
		creds, err := loadServerTLS(*tlsCert, *tlsKey, *tlsCA)
		if err != nil {
			log.Fatalf("Failed to initialize server TLS: %v", err)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		log.Printf("🔒 gRPC service secured with TLS transport credentials.")
	}
	if *apiToken != "" || tokenStore != nil {
		authInterceptor := api.NewAuthInterceptor(*apiToken, tokenStore)
		grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(authInterceptor.Unary()), grpc.StreamInterceptor(authInterceptor.Stream()))
		if *apiToken != "" {
			log.Printf("🔐 gRPC service protected with Bearer token authentication.")
		}
	}

	grpcServer := api.NewGRPCServer(engine, grpcOpts...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *role == "leader" {
		repServer := replication.NewLeaderServer(engine.WALPath())
		repServer.RegisterService(grpcServer.RawServer())
		engine.SetCommitObserver(repServer)
	} else {
		clientCreds, err := loadClientTLS(*tlsCert, *tlsKey, *tlsCA)
		if err != nil {
			log.Fatalf("Failed to initialize follower client TLS: %v", err)
		}
		conn, err := grpc.NewClient(*leaderAddr, grpc.WithTransportCredentials(clientCreds))
		if err != nil {
			log.Fatalf("Failed to dial leader node: %v", err)
		}
		defer conn.Close()
		followerClient := replication.NewFollowerClient(*replicaID, engine, conn)
		go func() {
			for ctx.Err() == nil {
				if err := followerClient.SyncLoop(ctx); err != nil && ctx.Err() == nil {
					log.Printf("Follower sync stream dropped: %v. Reconnecting in 1s...", err)
					select {
					case <-time.After(time.Second):
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	var partitions []handler.PartitionEntry
	cfgPath := *partitionConfig
	if cfgPath == "" {
		for _, c := range []string{"partitions.json", "beastdb-partitions.json", filepath.Join(*dataDir, "partitions.json")} {
			if _, err := os.Stat(c); err == nil {
				cfgPath = c
				break
			}
		}
	}
	if cfgPath != "" {
		if data, err := os.ReadFile(cfgPath); err == nil {
			if err := json.Unmarshal(data, &partitions); err == nil {
				log.Printf("[Studio] Loaded %d partition definitions from %s", len(partitions), cfgPath)
			} else {
				log.Printf("[Studio] Warning: failed to parse %s: %v", cfgPath, err)
			}
		} else {
			log.Printf("[Studio] Warning: failed to read %s: %v", cfgPath, err)
		}
	}

	stopWeb := startWebServer(*webAddr, *dataDir, *role, *adminPassword, engine, partitions, tokenStore)
	defer stopWeb()

	lis, err := net.Listen("tcp", grpcBind)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", grpcBind, err)
	}
	go func() {
		log.Printf("BeastDB gRPC server listening on %s", lis.Addr().String())
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server terminated: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("Received signal %s. Initiating graceful shutdown...", sig)

	cancel()
	grpcServer.GracefulStop()
	if err := engine.Close(); err != nil {
		log.Printf("Error flushing engine to disk: %v", err)
	}
	log.Println("BeastDB stopped cleanly. Goodbye!")
}
