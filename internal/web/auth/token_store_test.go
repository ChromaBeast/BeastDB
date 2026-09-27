package auth

import (
	"testing"
)

type mockEngine struct {
	data map[uint64][]byte
}

func newMockEngine() *mockEngine {
	return &mockEngine{data: make(map[uint64][]byte)}
}

func (m *mockEngine) Put(key uint64, val []byte) error {
	m.data[key] = val
	return nil
}

func (m *mockEngine) Get(key uint64) ([]byte, bool, error) {
	val, ok := m.data[key]
	return val, ok, nil
}

func (m *mockEngine) Delete(key uint64) error {
	delete(m.data, key)
	return nil
}

func TestTokenStoreCreateListDelete(t *testing.T) {
	engine := newMockEngine()
	ts := NewTokenStore(engine, func(start, end uint64) ([]TokenRecord, error) {
		var recs []TokenRecord
		for k, v := range engine.data {
			if k >= start && k <= end {
				recs = append(recs, TokenRecord{Key: k, Value: v})
			}
		}
		return recs, nil
	})

	// 1. Create token
	rawToken, tok, err := ts.Create("Test Token", "admin", "admin")
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}
	if rawToken == "" || tok.ID == "" {
		t.Fatalf("expected non-empty token and ID")
	}

	// 2. Validate token
	if !ts.Validate(rawToken) {
		t.Fatalf("expected raw token to validate")
	}
	if ts.Validate("invalid_token") {
		t.Fatalf("expected invalid token to fail")
	}

	// 3. List tokens
	tokens := ts.List()
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}

	// 4. Delete token
	if err := ts.Delete(tok.ID); err != nil {
		t.Fatalf("unexpected error deleting token: %v", err)
	}
	if ts.Validate(rawToken) {
		t.Fatalf("expected deleted token to no longer validate")
	}
	if len(ts.List()) != 0 {
		t.Fatalf("expected 0 tokens after deletion")
	}
}
