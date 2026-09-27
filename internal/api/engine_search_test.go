package api

import (
	"path/filepath"
	"testing"
)

func TestEngineSecondaryIndexSearch(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "search_test.bin")
	walPath := filepath.Join(tempDir, "search_test.wal")

	engine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. Put records with varying text
	records := map[uint64]string{
		100: `{"name": "Alice", "role": "admin", "bio": "Distributed systems engineer"}`,
		200: `{"name": "Bob", "role": "developer", "bio": "Frontend systems specialist"}`,
		300: `{"name": "Charlie", "role": "admin", "bio": "Security and cryptography researcher"}`,
		400: `{"name": "Diana", "role": "developer", "bio": "Database performance optimization"}`,
	}

	for k, v := range records {
		if err := engine.Put(k, []byte(v)); err != nil {
			t.Fatalf("put failed for key %d: %v", k, err)
		}
	}

	// 2. Search for "admin" -> should match 100 and 300
	results, err := engine.Search("admin", 10, 0)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 admin matches, got %d", len(results))
	}

	// 3. Multi-term intersection: "systems" AND "admin" -> should match 100 only
	results, err = engine.Search("systems admin", 10, 0)
	if err != nil {
		t.Fatalf("multi-term search error: %v", err)
	}
	if len(results) != 1 || results[0].Key != 100 {
		t.Fatalf("expected key 100 for 'systems admin', got %+v", results)
	}

	// 4. Delete record 100 and verify it is removed from secondary index
	if err := engine.Delete(100); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	results, err = engine.Search("systems admin", 10, 0)
	if err != nil {
		t.Fatalf("search after delete error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 matches after delete, got %d", len(results))
	}

	// 5. Cleanly close and reopen — index should rebuild from stored tuples
	if err := engine.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	reopened, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer reopened.Close()

	// "cryptography" should still match 300
	results, err = reopened.Search("cryptography", 10, 0)
	if err != nil {
		t.Fatalf("search after reopen error: %v", err)
	}
	if len(results) != 1 || results[0].Key != 300 {
		t.Fatalf("expected key 300 for 'cryptography', got %+v", results)
	}
}
