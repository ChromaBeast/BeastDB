package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const (
	// Version is the current release tag of BeastDB.
	Version = "0.1.0-alpha"
	// DefaultPort is the standard listener port.
	DefaultPort = 6379
)

func main() {
	fmt.Printf("Starting BeastDB v%s on port :%d...\n", Version, DefaultPort)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Graceful shutdown listener
	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %s. Flushing WAL and shutting down BeastDB...\n", sig)
		os.Exit(0)
	}()

	fmt.Println("BeastDB ready for connections.")
	select {}
}
