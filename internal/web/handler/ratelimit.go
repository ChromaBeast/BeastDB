package handler

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoginRateLimiter limits failed login attempts per client IP.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

// NewLoginRateLimiter creates a rate limiter for login attempts.
func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	rl := &LoginRateLimiter{
		attempts: make(map[string][]time.Time),
		max:      maxAttempts,
		window:   window,
	}
	go rl.cleanupLoop()
	return rl
}

// Allow checks if the given IP is allowed to attempt a login.
func (rl *LoginRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.attempts[ip] = valid

	return len(valid) < rl.max
}

// RecordFailure records a failed login attempt for the IP.
func (rl *LoginRateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[ip] = append(rl.attempts[ip], time.Now())
}

// ClientIP extracts the real client IP from remote address or proxy headers.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (rl *LoginRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for ip, times := range rl.attempts {
			var valid []time.Time
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.attempts, ip)
			} else {
				rl.attempts[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}
