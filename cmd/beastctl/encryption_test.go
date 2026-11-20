package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
)

func TestEncryptBackupVerifyRestoreDrill(t *testing.T) {
	dir := t.TempDir()
	db, wal := filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal")
	key, other := filepath.Join(dir, "k"), filepath.Join(dir, "other")
	if err := runKeygen(key); err != nil {
		t.Fatal(err)
	}
	if err := runKeygen(other); err != nil {
		t.Fatal(err)
	}

	eng, err := api.NewEngine(db, wal, 20)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 150; i++ {
		_ = eng.Put(i, []byte("drill-record-"+strings.Repeat("d", 40)))
	}
	_ = eng.Close()

	if err := runMigrate("Encrypt", db, wal, "", key, true); err != nil {
		t.Fatal(err)
	}
	// Backing up without the key fails; with it succeeds and records the key id.
	bundle := filepath.Join(dir, "bundle")
	if err := runBackupKey(db, wal, bundle, ""); err == nil {
		t.Fatal("backup of an encrypted database without a key must fail")
	}
	if err := runBackupKey(db, wal, bundle, key); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(bundle)
	if err != nil || !m.Encrypted || m.KeyID != 1 {
		t.Fatalf("manifest %+v %v", m, err)
	}
	if err := runVerifyKey(bundle, ""); err != nil { // checksum only
		t.Fatal(err)
	}
	if err := runVerifyKey(bundle, key); err != nil { // authenticates pages
		t.Fatal(err)
	}
	if err := runVerifyKey(bundle, other); err == nil {
		t.Fatal("verify with the wrong key must fail")
	}

	dest := filepath.Join(dir, "restored")
	if _, err := runRestoreKey(bundle, dest, false, ""); err == nil {
		t.Fatal("restoring an encrypted bundle without a key must fail")
	}
	if _, err := runRestoreKey(bundle, dest, false, other); err == nil {
		t.Fatal("restoring with the wrong key must fail")
	}
	if _, err := os.Stat(filepath.Join(dest, "beast.bin")); err == nil {
		t.Fatal("a failed restore must not leave a database behind")
	}
	res, err := runRestoreKey(bundle, dest, false, key)
	if err != nil || res.RecordCount != 150 {
		t.Fatalf("restore: %+v %v", res, err)
	}
	if !crypt.HasKeyring(res.DestDB) {
		t.Fatal("restored database has no keyring")
	}
}
