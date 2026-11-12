package auth

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTokenExpiryAndRole(t *testing.T) {
	engine := newMockEngine()
	scan := func(start, end uint64) ([]TokenRecord, error) {
		var recs []TokenRecord
		for k, v := range engine.data {
			if k >= start && k <= end {
				recs = append(recs, TokenRecord{Key: k, Value: v})
			}
		}
		return recs, nil
	}
	ts := NewTokenStore(engine, scan)

	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)
	expired, _, _ := ts.CreateWithExpiry("old", "viewer", "t", &past)
	live, _, _ := ts.CreateWithExpiry("new", "writer", "t", &future)
	forever, _, _ := ts.Create("forever", "viewer", "t")

	if ts.Validate(expired) {
		t.Fatal("expired token must not validate")
	}
	if role, ok := ts.ValidateRole(live); !ok || role != "writer" {
		t.Fatalf("live token: %q %v", role, ok)
	}
	if role, ok := ts.ValidateRole(forever); !ok || role != "viewer" {
		t.Fatalf("no-expiry token: %q %v", role, ok)
	}

	// Persisted and reloaded: expiry still enforced.
	ts2 := NewTokenStore(engine, scan)
	if ts2.Validate(expired) || !ts2.Validate(live) {
		t.Fatal("expiry not preserved across reload")
	}
}

func TestLegacyStoredTokenWithoutExpiry(t *testing.T) {
	raw := "bst_live_legacy"
	rec, _ := json.Marshal(map[string]any{"id": "tok_1", "name": "n", "token_hash": hashToken(raw), "role": "admin"})
	ts := NewTokenStore(newMockEngine(), func(start, end uint64) ([]TokenRecord, error) {
		return []TokenRecord{{Key: tokenKey("tok_1"), Value: rec}}, nil
	})
	if role, ok := ts.ValidateRole(raw); !ok || role != "admin" {
		t.Fatalf("legacy token should validate as admin, got %q %v", role, ok)
	}
}
