package main

import (
	"context"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const Version = "0.1.0-release"

func main() {
	role          := flag.String("role", "leader", "Node cluster role: leader or follower")
	port          := flag.Int("port", 50051, "Port for gRPC service")
	leaderAddr    := flag.String("leader-addr", "127.0.0.1:50051", "Leader node address for replication")
	dataDir       := flag.String("data-dir", "./data", "Directory to store data and WAL files")
	poolSize      := flag.Int("pool-size", 128, "Buffer pool frame capacity (4KB blocks)")
	replicaID     := flag.String("replica-id", "replica-1", "Unique identifier for this replica node")
	webAddr       := flag.String("web-addr", "127.0.0.1:8080", "Address for the web admin console (empty to disable)")
	adminPassword := flag.String("admin-password", "changeme", "Initial admin user password (used only on first run)")
	flag.Parse()

	log.Printf("Starting BeastDB v%s [Role: %s] on port :%d...", Version, *role, *port)

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	dbPath  := filepath.Join(*dataDir, "beast.bin")
	walPath := filepath.Join(*dataDir, "beast.wal")

	engine, err := api.NewEngine(dbPath, walPath, *poolSize)
	if err != nil {
		log.Fatalf("Failed to initialize database engine: %v", err)
	}

	grpcServer := api.NewGRPCServer(engine)
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

	// Start embedded web admin console if an address is configured.
	if *webAddr != "" {
		secret, secretErr := auth.GenerateSecret()
		if secretErr != nil {
			log.Fatalf("Failed to generate session secret: %v", secretErr)
		}

		webSrv, webErr := web.NewServer(*webAddr, engine, secret)
		if webErr != nil {
			log.Fatalf("Failed to create web server: %v", webErr)
		}

		// Seed the default admin user on first run (no-op if already exists).
		store := auth.NewUserStore(engine)
		if seedErr := store.CreateUser("admin", *adminPassword, auth.RoleAdmin); seedErr != nil {
			log.Printf("Admin user already exists (skipping seed): %v", seedErr)
		} else {
			log.Printf("Admin user created. Change the default password immediately!")
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
