package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/crypt"
)

func serverBundle(t *testing.T, dir string, opts api.Options, n int) string {
	t.Helper()
	eng, err := api.NewEngineWithOptions(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	for i := uint64(1); i <= uint64(n); i++ {
		if err := eng.Put(i, []byte(fmt.Sprintf("payload-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	info, err := backup.Create(eng, filepath.Join(dir, "backups"), backup.KindManual, "test")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "backups", info.ID)
}

func openRestored(t *testing.T, dir string, opts api.Options, n int) {
	t.Helper()
	e, err := api.NewEngineWithOptions(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32, opts)
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	defer e.Close()
	for i := uint64(1); i <= uint64(n); i++ {
		if v, ok, err := e.Get(i); err != nil || !ok || string(v) != fmt.Sprintf("payload-%d", i) {
			t.Fatalf("record %d: %q %v %v", i, v, ok, err)
		}
	}
}

func noStaging(t *testing.T, parent string) {
	t.Helper()
	ents, _ := os.ReadDir(parent)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".beastdb-restore-") {
			t.Fatalf("staging directory left behind: %s", e.Name())
		}
	}
}

func TestRestoreNewCreatesVerifiedDirectory(t *testing.T) {
	dir := t.TempDir()
	bundle := serverBundle(t, dir, api.Options{}, 120)
	target := filepath.Join(dir, "fresh", "data")
	if err := runRestoreNew(bundle, target, ""); err != nil {
		t.Fatal(err)
	}
	openRestored(t, target, api.Options{}, 120)
	noStaging(t, filepath.Dir(target))

	// An empty existing directory is acceptable (a freshly provisioned volume)...
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runRestoreNew(bundle, empty, ""); err != nil {
		t.Fatalf("empty dir: %v", err)
	}
	openRestored(t, empty, api.Options{}, 120)
}

func TestRestoreNewNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	bundle := serverBundle(t, dir, api.Options{}, 50)
	live := filepath.Join(dir, "live")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(live, "beast.bin")
	if err := os.WriteFile(marker, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runRestoreNew(bundle, live, ""); err == nil || !strings.Contains(err.Error(), "never overwrites") {
		t.Fatalf("a non-empty directory must be refused: %v", err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "precious" {
		t.Fatal("the existing directory was touched")
	}
	for _, bad := range []string{bundle, filepath.Join(bundle, "sub")} {
		if err := runRestoreNew(bundle, bad, ""); err == nil {
			t.Fatalf("target %s inside the bundle must be refused", bad)
		}
	}
	// A regular file in the way is refused too.
	f := filepath.Join(dir, "afile")
	_ = os.WriteFile(f, nil, 0o600)
	if err := runRestoreNew(bundle, f, ""); err == nil {
		t.Fatal("a file at the target must be refused")
	}
}

func TestRestoreNewCorruptBundleLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	bundle := serverBundle(t, dir, api.Options{}, 50)
	p := filepath.Join(bundle, "data.bin")
	b, _ := os.ReadFile(p)
	b[len(b)-1] ^= 0xFF
	_ = os.WriteFile(p, b, 0o600)
	target := filepath.Join(dir, "out")
	if err := runRestoreNew(bundle, target, ""); err == nil {
		t.Fatal("a corrupt bundle must not restore")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("target must not exist after a failed restore")
	}
	noStaging(t, dir)
}

func TestRestoreNewEncrypted(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "master.key")
	if err := crypt.GenerateKeyFile(key); err != nil {
		t.Fatal(err)
	}
	opts := api.Options{KeyFile: key}
	bundle := serverBundle(t, dir, opts, 80)
	target := filepath.Join(dir, "enc-new")
	if err := runRestoreNew(bundle, target, ""); err == nil {
		t.Fatal("an encrypted bundle needs the key")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("target created without a key")
	}
	if err := runRestoreNew(bundle, target, key); err != nil {
		t.Fatal(err)
	}
	openRestored(t, target, opts, 80)
	noStaging(t, dir)
}
