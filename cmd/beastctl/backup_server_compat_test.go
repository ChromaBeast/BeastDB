package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/backup"
)

// A backup made by the running server (internal/backup) must be accepted by the
// offline tools: restore is still a beastctl step.
func TestServerMadeBackupVerifiesAndRestoresWithBeastctl(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	const n = 150
	for i := uint64(1); i <= n; i++ {
		if err := eng.Put(i, []byte(fmt.Sprintf("payload-%d", i))); err != nil {
			t.Fatal(err)
		}
	}

	info, err := backup.Create(eng, filepath.Join(dir, "backups"), backup.KindManual, "test")
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "backups", info.ID)

	if err := runVerify(bundle); err != nil {
		t.Fatalf("beastctl verify rejected a server-made backup: %v", err)
	}
	res, err := runRestore(bundle, filepath.Join(dir, "restored"), false)
	if err != nil {
		t.Fatalf("beastctl restore failed on a server-made backup: %v", err)
	}
	restored, err := api.NewEngine(filepath.Join(dir, "restored", "beast.bin"), filepath.Join(dir, "restored", "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	for i := uint64(1); i <= n; i++ {
		v, ok, err := restored.Get(i)
		if err != nil || !ok || string(v) != fmt.Sprintf("payload-%d", i) {
			t.Fatalf("record %d after restore: %q %v %v", i, v, ok, err)
		}
	}
	_ = res
}
