package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// isLoopback determines if an address string resolves to a loopback interface.
func isLoopback(addr string) bool {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return true // not bound / disabled
	}

	host := trimmed
	if h, _, err := net.SplitHostPort(trimmed); err == nil {
		host = h
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		return false // wildcard bind to all external interfaces
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validateSecurityConfig ensures that default credentials are not deployed in production
// or exposed on public network interfaces.
func validateSecurityConfig(password, webAddr, grpcAddr string, devMode, insecureAuth bool) error {
	if devMode || insecureAuth {
		return nil
	}

	if password == "admin" {
		isProd := strings.EqualFold(os.Getenv("BEASTDB_ENV"), "production")
		webPublic := webAddr != "" && !isLoopback(webAddr)
		grpcPublic := grpcAddr != "" && !isLoopback(grpcAddr)

		if isProd || webPublic || grpcPublic {
			return fmt.Errorf("insecure credentials detected: default password 'admin' is prohibited on non-loopback listeners (web=%s, grpc=%s) or production environments (BEASTDB_ENV=production). Set BEASTDB_ADMIN_PASSWORD or pass --dev for local sandbox", webAddr, grpcAddr)
		}
	}

	return nil
}
