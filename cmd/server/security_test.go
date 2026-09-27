package main

import (
	"os"
	"testing"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:50051", true},
		{"localhost:8080", true},
		{"127.0.0.5:80", true},
		{"[::1]:50051", true},
		{"", true},
		{":50051", false},
		{"0.0.0.0:8080", false},
		{"192.168.1.100:50051", false},
		{"10.0.0.1:8080", false},
	}

	for _, tt := range tests {
		got := isLoopback(tt.addr)
		if got != tt.want {
			t.Errorf("isLoopback(%q) = %v; want %v", tt.addr, got, tt.want)
		}
	}
}

func TestValidateSecurityConfig(t *testing.T) {
	// Dev mode allows admin/admin
	if err := validateSecurityConfig("admin", "0.0.0.0:8080", "0.0.0.0:50051", true, false); err != nil {
		t.Fatalf("dev mode should allow default password: %v", err)
	}

	// Insecure flag allows admin/admin
	if err := validateSecurityConfig("admin", "0.0.0.0:8080", "0.0.0.0:50051", false, true); err != nil {
		t.Fatalf("insecure-auth flag should allow default password: %v", err)
	}

	// Loopback allows admin/admin with warning
	if err := validateSecurityConfig("admin", "127.0.0.1:8080", "127.0.0.1:50051", false, false); err != nil {
		t.Fatalf("loopback should allow admin/admin: %v", err)
	}

	// Public web listener rejects admin/admin
	if err := validateSecurityConfig("admin", "0.0.0.0:8080", "127.0.0.1:50051", false, false); err == nil {
		t.Fatal("expected error on public web listener with default admin")
	}

	// Public gRPC listener rejects admin/admin
	if err := validateSecurityConfig("admin", "127.0.0.1:8080", ":50051", false, false); err == nil {
		t.Fatal("expected error on public grpc listener with default admin")
	}

	// Production environment rejects admin/admin even on loopback
	os.Setenv("BEASTDB_ENV", "production")
	defer os.Unsetenv("BEASTDB_ENV")
	if err := validateSecurityConfig("admin", "127.0.0.1:8080", "127.0.0.1:50051", false, false); err == nil {
		t.Fatal("expected error in production env with default admin")
	}

	// Custom strong password passes
	if err := validateSecurityConfig("super-secret-password-123", "0.0.0.0:8080", ":50051", false, false); err != nil {
		t.Fatalf("custom password should pass: %v", err)
	}
}
