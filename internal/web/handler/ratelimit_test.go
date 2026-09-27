package handler

import (
	"testing"
	"time"
)

func TestLoginRateLimiter(t *testing.T) {
	rl := NewLoginRateLimiter(3, 100*time.Millisecond)

	ip := "192.168.1.100"
	if !rl.Allow(ip) {
		t.Fatalf("expected initial allow to be true")
	}

	rl.RecordFailure(ip)
	rl.RecordFailure(ip)
	if !rl.Allow(ip) {
		t.Fatalf("expected 2 failures to still be under limit of 3")
	}

	rl.RecordFailure(ip)
	if rl.Allow(ip) {
		t.Fatalf("expected 3 failures to trigger rate limit")
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)
	if !rl.Allow(ip) {
		t.Fatalf("expected rate limit to expire after window")
	}
}
