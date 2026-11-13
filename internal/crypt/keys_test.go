package crypt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyringCreateOpenWrongKeyAndTamper(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "beast.bin")
	master := make([]byte, KeySize)
	master[0] = 1
	k, err := CreateKeyring(db, master, 3)
	if err != nil || k.ID != 3 {
		t.Fatalf("create: %v", err)
	}
	if _, err := CreateKeyring(db, master, 4); err == nil {
		t.Fatal("must not replace an existing keyring")
	}
	again, err := OpenKeyring(db, master)
	if err != nil || again.ID != 3 {
		t.Fatalf("open: %v", err)
	}
	// Same DEK: what one seals the other opens.
	n := make([]byte, 12)
	ct := k.PageAEAD().Seal(nil, n, []byte("x"), nil)
	if _, err := again.PageAEAD().Open(nil, n, ct, nil); err != nil {
		t.Fatalf("keys differ across opens: %v", err)
	}
	// Page and WAL keys are different subkeys.
	if _, err := k.WALAEAD().Open(nil, n, ct, nil); err == nil {
		t.Fatal("page and WAL keys must differ")
	}
	wrong := make([]byte, KeySize)
	if _, err := OpenKeyring(db, wrong); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("want ErrWrongKey, got %v", err)
	}
	if id, err := KeyringKeyID(db); err != nil || id != 3 {
		t.Fatalf("key id: %d %v", id, err)
	}
	// Editing the key ID in the clear breaks the unwrap (it is in the AAD).
	b, _ := os.ReadFile(KeyringPath(db))
	edited := []byte(string(b))
	for i := range edited {
		if i+len(`"key_id": 3`) <= len(edited) && string(edited[i:i+len(`"key_id": 3`)]) == `"key_id": 3` {
			edited[i+len(`"key_id": 3`)-1] = '4'
		}
	}
	_ = os.WriteFile(KeyringPath(db), edited, 0o600)
	if _, err := OpenKeyring(db, master); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("edited key id: want ErrWrongKey, got %v", err)
	}
	_ = os.WriteFile(KeyringPath(db), []byte("{not json"), 0o600)
	if _, err := OpenKeyring(db, master); !errors.Is(err, ErrBadKeyring) {
		t.Fatalf("damaged keyring: want ErrBadKeyring, got %v", err)
	}
}
