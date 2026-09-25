package api

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPutIfAbsentPreservesExistingValue(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(filepath.Join(dir, "db.bin"), filepath.Join(dir, "db.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	const key = uint64(18446744073709551615)
	if err := engine.PutIfAbsent(key, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := engine.PutIfAbsent(key, []byte("second")); !errors.Is(err, ErrRecordExists) {
		t.Fatalf("expected conflict, got %v", err)
	}
	value, exists, err := engine.Get(key)
	if err != nil || !exists || string(value) != "first" {
		t.Fatalf("existing value changed: %q, %v, %v", value, exists, err)
	}
}
