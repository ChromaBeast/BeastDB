package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
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

// Re-wrap-only rotation: the data file and WAL are byte-identical afterwards,
// the old key stops opening the keyring, and backups, verify and restore work
// with the new key. A key supplied through env: and exec: sources works too.
func TestRewrapOnlyRotationDrill(t *testing.T) {
	dir := t.TempDir()
	db, wal := filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal")
	oldKey, newKey := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	for _, k := range []string{oldKey, newKey} {
		if err := runKeygen(k); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := api.NewEngine(db, wal, 20)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 80; i++ {
		_ = eng.Put(i, []byte("rewrap-"+strings.Repeat("r", 30)))
	}
	_ = eng.Close()
	if err := runMigrate("Encrypt", db, wal, "", oldKey, true); err != nil {
		t.Fatal(err)
	}
	dataBefore, _ := os.ReadFile(db)
	walBefore, _ := os.ReadFile(wal)

	if err := runRewrap(db, newKey, oldKey); err == nil {
		t.Fatal("rewrap with the wrong current key must fail")
	}
	if err := runRewrap(db, oldKey, oldKey); err == nil {
		t.Fatal("rewrap to the same key must be refused")
	}
	if err := runRewrap(db, oldKey, newKey); err != nil {
		t.Fatal(err)
	}
	dataAfter, _ := os.ReadFile(db)
	walAfter, _ := os.ReadFile(wal)
	if string(dataBefore) != string(dataAfter) || string(walBefore) != string(walAfter) {
		t.Fatal("rewrap-only must not touch the data file or the WAL")
	}
	if _, err := api.NewEngineWithOptions(db, wal, 20, api.Options{KeyFile: oldKey}); err == nil {
		t.Fatal("old key must no longer open the database")
	}

	// The new key opens it, from a file, from the environment and from a command.
	t.Setenv("BEASTDB_REWRAP_TEST_KEY", strings.TrimSpace(mustRead(t, newKey)))
	for _, spec := range []string{newKey, "env:BEASTDB_REWRAP_TEST_KEY"} {
		e, err := api.NewEngineWithOptions(db, wal, 20, api.Options{KeyFile: spec})
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if got := e.Encryption(); !got.Enabled || got.KeySource != crypt.SourceKind(spec) {
			t.Fatalf("encryption info %+v for %s", got, spec)
		}
		if v, _, err := e.Get(40); err != nil || len(v) == 0 {
			t.Fatalf("read after rewrap: %v", err)
		}
		_ = e.Close()
	}

	// Backups made after the rotation verify and restore with the new key only.
	bundle := filepath.Join(dir, "bundle")
	if err := runBackupKey(db, wal, bundle, newKey); err != nil {
		t.Fatal(err)
	}
	if err := runVerifyKey(bundle, newKey); err != nil {
		t.Fatal(err)
	}
	if err := runVerifyKey(bundle, oldKey); err == nil {
		t.Fatal("verify with the retired key must fail")
	}
	if _, err := runRestoreKey(bundle, filepath.Join(dir, "restored"), false, newKey); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The audit log beside the database follows it through encrypt, rekey and
// decrypt with its hash chain intact, and the Studio user record (stored in the
// engine's own keyspace) is not readable in the encrypted data file.
func TestAuditLogAndUserStoreFollowEncryption(t *testing.T) {
	dir := t.TempDir()
	db, wal := filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal")
	k1, k2 := filepath.Join(dir, "k1"), filepath.Join(dir, "k2")
	for _, k := range []string{k1, k2} {
		if err := runKeygen(k); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := api.NewEngine(db, wal, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.NewUserStore(eng).CreateUser("studio-operator-zq", "pw-zq-12345", auth.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	_ = eng.Close()
	al, err := audit.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_ = al.Record("studio-operator-zq", audit.ActionLoginOK, "", "", "127.0.0.1")
	}
	_ = al.Close()

	state := func(wantSealed bool, key string, want uint64) {
		t.Helper()
		var c *audit.Cipher
		if key != "" {
			master, _ := crypt.ReadMasterKey(key)
			keys, err := crypt.OpenKeyring(db, master)
			if err != nil {
				t.Fatal(err)
			}
			c = audit.NewCipher(keys.AuditAEAD(), keys.ID)
		}
		l, err := audit.OpenSealed(dir, c)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		if st := l.Verify(); !st.Verified || st.Entries != want || st.Encrypted != wantSealed {
			t.Fatalf("audit status %+v (want %d entries, sealed=%v)", st, want, wantSealed)
		}
	}
	state(false, "", 4)

	if err := runMigrate("Encrypt", db, wal, "", k1, true); err != nil {
		t.Fatal(err)
	}
	state(true, k1, 4)
	raw, _ := os.ReadFile(filepath.Join(dir, "audit.log"))
	if strings.Contains(string(raw), "login.ok") || strings.Contains(string(raw), "studio-operator-zq") {
		t.Fatal("audit log is readable after encrypt")
	}
	if data, _ := os.ReadFile(db); strings.Contains(string(data), "studio-operator-zq") || strings.Contains(string(data), "argon2") {
		t.Fatal("user record is readable in the encrypted data file")
	}
	e, err := api.NewEngineWithOptions(db, wal, 20, api.Options{KeyFile: k1})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := auth.NewUserStore(e).Authenticate("studio-operator-zq", "pw-zq-12345"); err != nil || u.Username != "studio-operator-zq" {
		t.Fatalf("user after encrypt: %v", err)
	}
	_ = e.Close()

	// A new data key (full rekey) re-seals the audit log under the new keys.
	if err := runMigrate("Rekey", db, wal, k1, k2, true); err != nil {
		t.Fatal(err)
	}
	state(true, k2, 4)

	// Re-wrap-only keeps the data key, so the log needs no change.
	k3 := filepath.Join(dir, "k3")
	_ = runKeygen(k3)
	if err := runRewrap(db, k2, k3); err != nil {
		t.Fatal(err)
	}
	state(true, k3, 4)

	if err := runMigrate("Decrypt", db, wal, k3, "", true); err != nil {
		t.Fatal(err)
	}
	state(false, "", 4)
	last, err := audit.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = last.Close() // Windows cannot remove an open file during TempDir cleanup
}
