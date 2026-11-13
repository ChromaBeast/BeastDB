package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

func newEncryptedEngine(t *testing.T, records int) (*api.Engine, string) {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "master.key")
	if err := crypt.GenerateKeyFile(key); err != nil {
		t.Fatal(err)
	}
	e, err := api.NewEngineWithOptions(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32, api.Options{KeyFile: key})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	for i := uint64(1); i <= uint64(records); i++ {
		if err := e.Put(i, []byte("BACKUP-SECRET-"+strings.Repeat("x", 50))); err != nil {
			t.Fatal(err)
		}
	}
	return e, key
}

func TestEncryptedBackupIsCiphertextAndTamperIsCaughtWithKey(t *testing.T) {
	e, keyPath := newEncryptedEngine(t, 300)
	root := filepath.Join(t.TempDir(), "backups")
	info, err := Create(e, root, KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Encrypted || info.KeyID != 1 || info.RootPageID == 0 {
		t.Fatalf("manifest %+v", info)
	}
	dir := filepath.Join(root, info.ID)
	data, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
	if bytes.Contains(data, []byte("BACKUP-SECRET")) {
		t.Fatal("backup data.bin holds plaintext")
	}
	kr, err := os.ReadFile(filepath.Join(dir, KeyringName))
	if err != nil || bytes.Contains(kr, []byte("master")) {
		t.Fatalf("keyring.json: %v", err)
	}

	// Without a key only checksum and size can be checked; with the key every page is.
	if _, err := Verify(dir); err != nil {
		t.Fatalf("keyless verify: %v", err)
	}
	master, _ := crypt.ReadMasterKey(keyPath)
	keys, err := crypt.OpenKeyringFile(filepath.Join(dir, KeyringName), master)
	if err != nil {
		t.Fatal(err)
	}
	pc := storage.NewPageCipher(keys.PageAEAD(), keys.ID)
	if _, err := VerifyWith(dir, pc); err != nil {
		t.Fatalf("keyed verify: %v", err)
	}

	// An attacker who flips a page bit and also rewrites the manifest checksum
	// defeats the keyless check but not the authenticated one.
	data[storage.PageSize*2+10] ^= 0x01
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	mb, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var m Manifest
	_ = json.Unmarshal(mb, &m)
	m.DataChecksumSHA256 = hex.EncodeToString(sum[:])
	mb, _ = json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o600)
	if _, err := Verify(dir); err != nil {
		t.Fatalf("keyless verify should still pass with a forged manifest: %v", err)
	}
	if _, err := VerifyWith(dir, pc); err == nil {
		t.Fatal("authenticated verify must reject the flipped page")
	}

	// A different master key cannot open the bundle's keyring.
	otherKey := filepath.Join(t.TempDir(), "other")
	_ = crypt.GenerateKeyFile(otherKey)
	om, _ := crypt.ReadMasterKey(otherKey)
	if _, err := crypt.OpenKeyringFile(filepath.Join(dir, KeyringName), om); err != crypt.ErrWrongKey {
		t.Fatalf("want ErrWrongKey, got %v", err)
	}
}

func TestManagerVerifiesEncryptedBackupWithEngineKey(t *testing.T) {
	e, _ := newEncryptedEngine(t, 100)
	m := NewManager(e, filepath.Join(t.TempDir(), "backups"))
	info, err := m.Create(KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.Verify(info.ID)
	if err != nil || !res.OK {
		t.Fatalf("manager verify: %+v %v", res, err)
	}
	p, _ := m.Path(info.ID)
	data, _ := os.ReadFile(filepath.Join(p, "data.bin"))
	data[storage.PageSize+5] ^= 1
	_ = os.WriteFile(filepath.Join(p, "data.bin"), data, 0o600)
	if res, _ := m.Verify(info.ID); res.OK {
		t.Fatal("tampered bundle verified")
	}
}
