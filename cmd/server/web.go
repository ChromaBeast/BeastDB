package main

import (
	"log"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/web"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

// startWebServer boots the HTTP admin console and seeds or synchronizes the admin credentials.
func startWebServer(webAddr, dataDir, role, adminPassword string, engine *api.Engine, partitions []handler.PartitionEntry, tokenStore *auth.TokenStore) func() {
	if webAddr == "" {
		return func() {}
	}

	secret, secretErr := loadOrGenerateSessionSecret(dataDir)
	if secretErr != nil {
		log.Fatalf("Failed to initialize session secret: %v", secretErr)
	}

	webSrv, webErr := web.NewServer(webAddr, engine, secret, role, Version, partitions, tokenStore)
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
