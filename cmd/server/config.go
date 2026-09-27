package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// getEnv returns the environment variable value if non-empty, or the fallback.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

// getEnvInt returns the integer environment variable or the fallback.
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// loadOrGenerateSessionSecret retrieves secret from env or persists to <dataDir>/session.key.
func loadOrGenerateSessionSecret(dataDir string) ([]byte, error) {
	// 1. Check BEASTDB_SESSION_SECRET env var
	if envSecret := os.Getenv("BEASTDB_SESSION_SECRET"); envSecret != "" {
		if decoded, err := hex.DecodeString(envSecret); err == nil && len(decoded) >= 32 {
			return decoded, nil
		}
		if len(envSecret) >= 32 {
			return []byte(envSecret[:32]), nil
		}
		log.Printf("Warning: BEASTDB_SESSION_SECRET should be at least 32 characters")
	}

	// 2. Check <dataDir>/session.key file
	keyPath := filepath.Join(dataDir, "session.key")
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		return data[:32], nil
	}

	// 3. Generate fresh cryptographically secure 32-byte secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}

	// Persist to session.key with restricted 0600 permissions
	if err := os.WriteFile(keyPath, secret, 0600); err != nil {
		log.Printf("Warning: could not persist session.key to %s: %v", keyPath, err)
	} else {
		log.Printf("Initialized and persisted session secret to %s", keyPath)
	}

	return secret, nil
}
