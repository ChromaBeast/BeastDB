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
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/logbuf"
	"github.com/ChromaBeast/beastdb/internal/replication"
	"github.com/ChromaBeast/beastdb/internal/runtimecfg"
	"github.com/ChromaBeast/beastdb/internal/web"
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
	walCheckpointMB := flag.Int("wal-checkpoint-mb", getEnvInt("BEASTDB_WAL_CHECKPOINT_MB", 64), "Checkpoint and rotate the WAL once it exceeds this many MiB")
	flag.Parse()

	// Tee the std logger into an in-memory tail for Studio; stderr output is unchanged.
	logs := logbuf.New(logbuf.DefaultCapacity)
	for _, secret := range []string{*apiToken, os.Getenv("BEASTDB_METRICS_TOKEN"), nonDefault(*adminPassword)} {
		logs.AddSecret(secret)
	}
	log.SetOutput(logs.Tee(os.Stderr))

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

	ckptCtx, stopCkpt := context.WithCancel(context.Background())
	defer stopCkpt()
	engine.StartAutoCheckpoint(ckptCtx, 10*time.Second, int64(*walCheckpointMB)<<20)

	// Settings an admin can change in Studio without a restart; saved values
	// in runtime-settings.json override the matching flags.
	runtimeCfg, rtErr := runtimecfg.Load(*dataDir, runtimecfg.Values{WALCheckpointMB: *walCheckpointMB, LoginMaxAttempts: 5, LoginWindowSeconds: 30, BackupKeep: 7})
	if rtErr != nil {
		log.Printf("Warning: ignoring runtime settings file: %v", rtErr)
	}
	runtimeCfg.Subscribe(func(v runtimecfg.Values) { engine.SetCheckpointThreshold(int64(v.WALCheckpointMB) << 20) })

	// Server-managed backups (create, verify, download, optional schedule).
	backups := backup.NewManager(engine, filepath.Join(*dataDir, "backups"))
	runtimeCfg.Subscribe(func(v runtimecfg.Values) {
		backups.Configure(time.Duration(v.BackupIntervalMinutes)*time.Minute, v.BackupKeep)
	})
	backups.Start(ckptCtx)

	tokenStore := initTokenStore(engine)
	grpcOpts := api.MessageSizeServerOptions()
	// Metrics interceptors run first so they also observe auth rejections.
	grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(api.MetricsUnaryInterceptor()), grpc.ChainStreamInterceptor(api.MetricsStreamInterceptor()))
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
		grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(authInterceptor.Unary()), grpc.ChainStreamInterceptor(authInterceptor.Stream()))
		if *apiToken != "" {
			log.Printf("🔐 gRPC service protected with Bearer token authentication.")
		}
	}

	grpcServer := api.NewGRPCServer(engine, grpcOpts...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Every node serves the replication service; it hands out WAL only while the
	// node is a leader (or a fenced former leader). The persisted epoch and role
	// in <data-dir>/cluster-state.json decide the starting role, so a restart
	// cannot undo a promotion or un-fence an old leader.
	flagRole := replication.RoleFollower
	if *role == "leader" {
		flagRole = replication.RoleLeader
	}
	repServer := replication.NewLeaderServer(engine.WALPath())
	repServer.RegisterService(grpcServer.RawServer())
	cluster, err := replication.NewCluster(engine, *dataDir, *replicaID, flagRole, repServer)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	replicaSource = repServer
	clusterControl = cluster
	switch cluster.Role() {
	case replication.RoleFollower:
		log.Printf("🔒 Node running in follower mode at epoch %d: engine write path set to read-only.", cluster.Epoch())
		clientCreds, err := loadClientTLS(*tlsCert, *tlsKey, *tlsCA)
		if err != nil {
			log.Fatalf("Failed to initialize follower client TLS: %v", err)
		}
		conn, err := grpc.NewClient(*leaderAddr, grpc.WithTransportCredentials(clientCreds), api.MessageSizeDialOption())
		if err != nil {
			log.Fatalf("Failed to dial leader node: %v", err)
		}
		defer conn.Close()
		cluster.RunFollower(ctx, replication.NewFollowerClient(*replicaID, engine, conn))
	case replication.RoleFenced:
		log.Printf("🔒 Node was fenced at epoch %d: it stays read-only. Wipe its data directory to rejoin it as a follower.", cluster.Epoch())
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

	logs.AddSecret(nonDefault(*adminPassword)) // dev mode generates its password after flag parsing
	webOpts := web.Options{
		Logs:    logs,
		Runtime: runtimeCfg,
		Backups: backups,
		Settings: handler.RuntimeSettings{
			GRPCAddr: grpcBind, WebAddr: *webAddr, DataDir: *dataDir, PoolSize: *poolSize,
			WALCheckpointMB: *walCheckpointMB, ReplicaID: *replicaID, LeaderAddr: *leaderAddr,
			TLSEnabled: *tlsCert != "" && *tlsKey != "", MutualTLS: *tlsCA != "",
			APITokenConfigured: *apiToken != "", MetricsTokenSet: os.Getenv("BEASTDB_METRICS_TOKEN") != "",
			PartitionConfigPath: cfgPath, DevMode: *devMode, InsecureAuthOverride: *insecureAuth,
		},
	}
	webOpts.Cluster = clusterControl
	stopWeb := startWebServer(*webAddr, *dataDir, *role, *adminPassword, engine, partitions, tokenStore, webOpts, *apiToken, os.Getenv("BEASTDB_METRICS_TOKEN"))
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
	stopCkpt()
	grpcServer.GracefulStop()
	if err := engine.Close(); err != nil {
		log.Printf("Error flushing engine to disk: %v", err)
	}
	log.Println("BeastDB stopped cleanly. Goodbye!")
}

// nonDefault returns "" for the well-known default password so ordinary log
// text containing the word is not masked; any real password is redacted.
func nonDefault(pw string) string {
	if pw == "admin" {
		return ""
	}
	return pw
}
