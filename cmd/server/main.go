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

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/replication"
	"github.com/ChromaBeast/beastdb/internal/web"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const Version = "0.1.0-release"

func main() {
	role              := flag.String("role", getEnv("BEASTDB_ROLE", "leader"), "Node cluster role: leader or follower")
	port              := flag.Int("port", getEnvInt("BEASTDB_PORT", 50051), "Port for gRPC service")
	leaderAddr        := flag.String("leader-addr", getEnv("BEASTDB_LEADER_ADDR", "127.0.0.1:50051"), "Leader node address for replication")
	dataDir           := flag.String("data-dir", getEnv("BEASTDB_DATA_DIR", "./data"), "Directory to store data and WAL files")
	poolSize          := flag.Int("pool-size", getEnvInt("BEASTDB_POOL_SIZE", 128), "Buffer pool frame capacity (4KB blocks)")
	replicaID         := flag.String("replica-id", getEnv("BEASTDB_REPLICA_ID", "replica-1"), "Unique identifier for this replica node")
	webAddr           := flag.String("web-addr", getEnv("BEASTDB_WEB_ADDR", "127.0.0.1:8080"), "Address for the web admin console (empty to disable)")
	adminPassword     := flag.String("admin-password", getEnv("BEASTDB_ADMIN_PASSWORD", "admin"), "Initial admin user password (or BEASTDB_ADMIN_PASSWORD)")
	partitionConfig   := flag.String("partition-config", getEnv("BEASTDB_PARTITION_CONFIG", ""), "Path to partitions.json defining Studio partition labels (optional)")
	apiToken          := flag.String("api-token", getEnv("BEASTDB_API_TOKEN", ""), "Bearer token for gRPC authentication (or BEASTDB_API_TOKEN)")
	devMode           := flag.Bool("dev", false, "Run in ephemeral emulator mode with temporary storage auto-purged on exit")
	flag.Parse()

	if *devMode {
		cleanup := setupDevMode(dataDir, webAddr, adminPassword, *port)
		defer cleanup()
	} else if *adminPassword == "admin" {
		log.Println("⚠️  SECURITY WARNING: Using default password 'admin'. Set BEASTDB_ADMIN_PASSWORD in production!")
	}

	log.Printf("Starting BeastDB v%s [Role: %s] on port :%d...", Version, *role, *port)

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	dbPath := filepath.Join(*dataDir, "beast.bin")
	walPath := filepath.Join(*dataDir, "beast.wal")

	engine, err := api.NewEngine(dbPath, walPath, *poolSize)
	if err != nil {
		log.Fatalf("Failed to initialize database engine: %v", err)
	}

	var grpcOpts []grpc.ServerOption
	if *apiToken != "" {
		authInterceptor := api.NewAuthInterceptor(*apiToken)
		grpcOpts = append(grpcOpts,
			grpc.UnaryInterceptor(authInterceptor.Unary()),
			grpc.StreamInterceptor(authInterceptor.Stream()),
		)
		log.Printf("🔐 gRPC service protected with Bearer token authentication.")
	} else if !*devMode {
		log.Printf("⚠️  SECURITY NOTICE: gRPC running without api-token. Set BEASTDB_API_TOKEN to require Bearer auth.")
	}

	grpcServer := api.NewGRPCServer(engine, grpcOpts...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *role == "leader" {
		repServer := replication.NewLeaderServer(engine.WALPath())
		repServer.RegisterService(grpcServer.RawServer())
		log.Printf("Leader node initialized with replication service.")
	} else {
		log.Printf("Follower node connecting to leader at %s...", *leaderAddr)
		conn, err := grpc.NewClient(*leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to dial leader node: %v", err)
		}
		defer conn.Close()

		followerClient := replication.NewFollowerClient(*replicaID, engine, conn)
		go func() {
			if err := followerClient.SyncLoop(ctx); err != nil && ctx.Err() == nil {
				log.Printf("Follower sync loop error: %v", err)
			}
		}()
	}

	// Load optional partition registry for the Studio UI.
	var partitions []handler.PartitionEntry
	if *partitionConfig != "" {
		data, readErr := os.ReadFile(*partitionConfig)
		if readErr != nil {
			log.Printf("Warning: could not read partition config %q: %v", *partitionConfig, readErr)
		} else if err := json.Unmarshal(data, &partitions); err != nil {
			log.Printf("Warning: invalid partition config JSON: %v", err)
		} else {
			log.Printf("Loaded %d partition definitions from %q", len(partitions), *partitionConfig)
		}
	}

	// Start embedded web admin console if an address is configured.
	if *webAddr != "" {
		secret, secretErr := loadOrGenerateSessionSecret(*dataDir)
		if secretErr != nil {
			log.Fatalf("Failed to initialize session secret: %v", secretErr)
		}

		webSrv, webErr := web.NewServer(*webAddr, engine, secret, *role, Version, partitions)
		if webErr != nil {
			log.Fatalf("Failed to create web server: %v", webErr)
		}

		// Seed default admin on first run, or synchronize if custom password provided.
		store := auth.NewUserStore(engine)
		if *adminPassword != "admin" {
			if err := store.SetUserPassword("admin", *adminPassword, auth.RoleAdmin); err != nil {
				log.Printf("Failed to sync admin password: %v", err)
			} else {
				log.Printf("Admin password synchronized with configured runtime flag.")
			}
		} else {
			if seedErr := store.CreateUser("admin", "admin", auth.RoleAdmin); seedErr != nil {
				log.Printf("Admin user already exists (skipping seed): %v", seedErr)
			} else {
				log.Printf("Default admin user created. Change the password immediately!")
			}
		}

		go webSrv.Serve()
		defer webSrv.Shutdown()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", *port, err)
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
	} else {
		log.Printf("Buffer pool and WAL flushed successfully to %s", *dataDir)
	}

	log.Println("BeastDB stopped cleanly. Goodbye!")
}
