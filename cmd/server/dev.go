package main

import (
	"fmt"
	"log"
	"os"
)

// setupDevMode initializes an ephemeral directory and returns cleanup func.
func setupDevMode(dataDir *string, webAddr *string, adminPassword *string, port int) func() {
	tmpDir, err := os.MkdirTemp("", "beastdb-dev-*")
	if err != nil {
		log.Fatalf("Failed to create temporary data directory: %v", err)
	}
	*dataDir = tmpDir

	if *adminPassword == "admin" {
		*adminPassword = "admin"
	}
	if *webAddr == "" {
		*webAddr = "127.0.0.1:8088"
	}

	printDevBanner(port, *webAddr, *adminPassword, tmpDir)

	return func() {
		log.Printf("[Dev Emulator] Cleaning up temporary storage: %s", tmpDir)
		_ = os.RemoveAll(tmpDir)
	}
}

func printDevBanner(grpcPort int, webAddr, password, tmpDir string) {
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⚡ BeastDB Dev Emulator (Ephemeral In-Memory Mode)          │")
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Printf("│  • gRPC Service:    127.0.0.1:%-30d│\n", grpcPort)
	if webAddr != "" {
		fmt.Printf("│  • Studio Console:  http://%-32s│\n", webAddr)
		fmt.Printf("│  • Credentials:     admin / %-31s│\n", password)
	}
	fmt.Printf("│  • Ephemeral Dir:   %-40s│\n", truncatePath(tmpDir, 40))
	fmt.Println("│  • Auto-cleanup:    All data is purged on process termination│")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

func truncatePath(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-(max-3):]
}
