package main

import (
	"log"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

// startWebServer boots the HTTP admin console and seeds or synchronizes the admin credentials.
// replicaSource is set by main on a leader so the web console can list replicas.
// It stays nil on followers (they do not see other replicas).
var replicaSource handler.ReplicationSource

func startWebServer(webAddr, dataDir, role, adminPassword string, engine *api.Engine, partitions []handler.PartitionEntry, tokenStore *auth.TokenStore, extra web.Options, metricsTokens ...string) func() {
	if webAddr == "" {
		return func() {}
	}

	secret, secretErr := loadOrGenerateSessionSecret(dataDir)
	if secretErr != nil {
		log.Fatalf("Failed to initialize session secret: %v", secretErr)
	}

	webSrv, webErr := web.NewServerWithOptions(webAddr, engine, secret, role, Version, partitions, withBase(withReplication(extra), dataDir, metricsTokens), tokenStore)
	if webErr != nil {
		log.Fatalf("Failed to create web server: %v", webErr)
	}

	store := auth.NewUserStore(engine)
	if adminPassword != "admin" {
		if err := store.SetUserPassword("admin", adminPassword, auth.RoleAdmin); err != nil {
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
	return func() {
		webSrv.Shutdown()
	}
}

// withBase fills the always-present options on top of the runtime settings and log buffer from main.
func withBase(o web.Options, dataDir string, metricsTokens []string) web.Options {
	o.DataDir = dataDir
	o.MetricsTokens = metricsTokens
	return o
}

// withReplication attaches the leader's replica source (nil on followers).
func withReplication(o web.Options) web.Options {
	o.Replication = replicaSource
	return o
}
