package api

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

const secretMarker = "TOP-SECRET-MARKER-0123456789"

func keyFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := crypt.GenerateKeyFile(p); err != nil {
		t.Fatal(err)
	}
	return p
}

type encEnv struct {
	dir, db, wal, key string
}

func newEncEnv(t *testing.T) encEnv {
	dir := t.TempDir()
	return encEnv{dir: dir, db: filepath.Join(dir, "beast.bin"), wal: filepath.Join(dir, "beast.wal"), key: keyFile(t, dir, "master.key")}
}

func (e encEnv) open(t *testing.T, pool int) *Engine {
	t.Helper()
	eng, err := NewEngineWithOptions(e.db, e.wal, pool, Options{KeyFile: e.key})
	if err != nil {
		t.Fatalf("open encrypted engine: %v", err)
	}
	return eng
}

func value(i int) []byte { return []byte(fmt.Sprintf("%s-%d-%s", secretMarker, i, strings.Repeat("v", 40))) }

func fillEngine(t *testing.T, eng *Engine, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if err := eng.Put(uint64(i), value(i)); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
}

func checkEngine(t *testing.T, eng *Engine, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		got, ok, err := eng.Get(uint64(i))
		if err != nil || !ok || !bytes.Equal(got, value(i)) {
			t.Fatalf("key %d: ok=%v err=%v got=%q", i, ok, err, got)
		}
	}
}

func fileHasMarker(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		t.Fatal(err)
	}
	return bytes.Contains(b, []byte(secretMarker))
}

func TestEncryptedEngineRoundTripLeavesNoPlaintext(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 16) // small pool: pages spill before the checkpoint
	if info := eng.Encryption(); !info.Enabled || info.Cipher != crypt.CipherName || info.KeyID != 1 {
		t.Fatalf("Encryption() = %+v", info)
	}
	fillEngine(t, eng, 600)
	big := bytes.Repeat([]byte(secretMarker), 20000) // multi-page value (ADR 0001)
	if err := eng.Put(9000, big); err != nil {
		t.Fatal(err)
	}
	// While running: the data file, journal, spill file and WAL hold no plaintext.
	for _, p := range []string{env.db, storage.JournalPath(env.db), storage.SpillPath(env.db), env.wal} {
		if fileHasMarker(t, p) {
			t.Fatalf("%s contains plaintext while the engine is running", filepath.Base(p))
		}
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{env.db, env.wal, env.wal + ".bak"} {
		if fileHasMarker(t, p) {
			t.Fatalf("%s contains plaintext after close", filepath.Base(p))
		}
	}

	re := env.open(t, 16)
	defer re.Close()
	checkEngine(t, re, 600)
	if got, ok, err := re.Get(9000); err != nil || !ok || !bytes.Equal(got, big) {
		t.Fatalf("large value after reopen: ok=%v err=%v", ok, err)
	}
	if !re.Encryption().Enabled {
		t.Fatal("reopened engine does not report encryption")
	}
}

func TestPlaintextEngineReportsEncryptionOff(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewEngine(filepath.Join(dir, "b.bin"), filepath.Join(dir, "b.wal"), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	if eng.Encryption().Enabled || eng.WALCipher() != nil || eng.KeyringPath() != "" {
		t.Fatalf("plaintext engine claims encryption: %+v", eng.Encryption())
	}
}

// copyDir copies the database files as a crash would leave them: no Close, no
// final checkpoint, spill file and journal as they stand.
func crashCopy(t *testing.T, env encEnv) encEnv {
	t.Helper()
	dst := t.TempDir()
	for _, name := range []string{"beast.bin", "beast.wal", "beast.wal.bak", "beast.bin.keyring", "beast.bin-journal", "master.key"} {
		b, err := os.ReadFile(filepath.Join(env.dir, name))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dst, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return encEnv{dir: dst, db: filepath.Join(dst, "beast.bin"), wal: filepath.Join(dst, "beast.wal"), key: filepath.Join(dst, "master.key")}
}

func TestEncryptedCrashRecovery(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 16)
	fillEngine(t, eng, 400)
	if err := eng.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	for i := 401; i <= 900; i++ { // after the checkpoint: only the WAL (and spill) hold these
		if err := eng.Put(uint64(i), value(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Delete(7); err != nil {
		t.Fatal(err)
	}
	if err := eng.BatchWrite([]BatchOperation{{Type: BatchOpPut, Key: 5000, Value: value(5000)}, {Type: BatchOpDelete, Key: 8}}); err != nil {
		t.Fatal(err)
	}

	crashed := crashCopy(t, env) // eng is never closed: this is the crash image
	rec := crashed.open(t, 16)
	defer rec.Close()
	for i := 1; i <= 900; i++ {
		got, ok, err := rec.Get(uint64(i))
		if err != nil {
			t.Fatalf("key %d: %v", i, err)
		}
		if i == 7 || i == 8 {
			if ok {
				t.Fatalf("deleted key %d came back", i)
			}
			continue
		}
		if !ok || !bytes.Equal(got, value(i)) {
			t.Fatalf("key %d lost in recovery: ok=%v got=%q", i, ok, got)
		}
	}
	if got, ok, _ := rec.Get(5000); !ok || !bytes.Equal(got, value(5000)) {
		t.Fatal("batch lost in recovery")
	}
	_ = eng.Close()
}

func TestEncryptedCrashWithTornWALTail(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 50)
	crashed := crashCopy(t, env)
	// A torn final frame: half of a plausible frame appended.
	f, err := os.OpenFile(crashed.wal, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(bytes.Repeat([]byte{0x41}, 30))
	_ = f.Close()
	rec := crashed.open(t, 32)
	defer rec.Close()
	checkEngine(t, rec, 50)
	_ = eng.Close()
}

func TestWrongMissingAndUnneededKey(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 20)
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	other := keyFile(t, env.dir, "other.key")
	if _, err := NewEngineWithOptions(env.db, env.wal, 32, Options{KeyFile: other}); !errors.Is(err, crypt.ErrWrongKey) {
		t.Fatalf("wrong key: want ErrWrongKey, got %v", err)
	}
	if _, err := NewEngine(env.db, env.wal, 32); !errors.Is(err, storage.ErrEncryptedNoKey) {
		t.Fatalf("no key: want ErrEncryptedNoKey, got %v", err)
	}
	// A failed open must not have damaged anything.
	re := env.open(t, 32)
	checkEngine(t, re, 20)
	_ = re.Close()

	// A key against a plaintext database is refused, never silently ignored,
	// and no keyring is created beside it.
	dir := t.TempDir()
	pdb, pwal := filepath.Join(dir, "p.bin"), filepath.Join(dir, "p.wal")
	pe, err := NewEngine(pdb, pwal, 32)
	if err != nil {
		t.Fatal(err)
	}
	_ = pe.Put(1, []byte("x"))
	_ = pe.Close()
	if _, err := NewEngineWithOptions(pdb, pwal, 32, Options{KeyFile: keyFile(t, dir, "k")}); !errors.Is(err, storage.ErrNotEncrypted) {
		t.Fatalf("key on plaintext: want ErrNotEncrypted, got %v", err)
	}
	if crypt.HasKeyring(pdb) {
		t.Fatal("a keyring was created beside a plaintext database")
	}

	// Lost keyring.
	if err := os.Remove(crypt.KeyringPath(env.db)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngineWithOptions(env.db, env.wal, 32, Options{KeyFile: env.key}); !errors.Is(err, crypt.ErrBadKeyring) {
		t.Fatalf("lost keyring: want ErrBadKeyring, got %v", err)
	}
}

func TestKeyFilePermissionsAndFormat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "k")
	if err := os.WriteFile(p, []byte(strings.Repeat("ab", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := crypt.ReadMasterKey(p); err == nil || !errors.Is(err, crypt.ErrKeyFilePerm) {
		t.Skipf("permission check not enforced on this platform: %v", err)
	}
	_ = os.Chmod(p, 0o600)
	if k, err := crypt.ReadMasterKey(p); err != nil || len(k) != 32 {
		t.Fatalf("hex key: %v", err)
	}
	_ = os.WriteFile(p, []byte("too short"), 0o600)
	if _, err := crypt.ReadMasterKey(p); !errors.Is(err, crypt.ErrBadKeyFile) {
		t.Fatalf("want ErrBadKeyFile, got %v", err)
	}
}

func flipByte(t *testing.T, path string, off int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var b [1]byte
	if _, err := f.ReadAt(b[:], off); err != nil {
		t.Fatal(err)
	}
	b[0] ^= 0x01
	if _, err := f.WriteAt(b[:], off); err != nil {
		t.Fatal(err)
	}
}

func TestTamperedDataPageIsDetected(t *testing.T) {
	for _, tc := range []struct {
		name string
		off  func(pageSize int64) int64
	}{
		{"ciphertext", func(ps int64) int64 { return ps*2 + 100 }},
		{"tag", func(ps int64) int64 { return ps*2 + storage.PageDataEnd + 3 }},
		{"nonce", func(ps int64) int64 { return ps*2 + storage.PageDataEnd + 20 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newEncEnv(t)
			eng := env.open(t, 32)
			fillEngine(t, eng, 300)
			if err := eng.Close(); err != nil {
				t.Fatal(err)
			}
			flipByte(t, env.db, tc.off(storage.PageSize))
			re, err := NewEngineWithOptions(env.db, env.wal, 32, Options{KeyFile: env.key})
			if err == nil {
				defer re.Close()
				for i := 1; i <= 300 && err == nil; i++ {
					_, _, err = re.Get(uint64(i))
				}
				// B+ tree pages are read on scan too
				if err == nil {
					_, err = re.Scan(0, ^uint64(0))
					if err == nil {
						_, _, _, err = re.ScanRecordsBounded(0, ^uint64(0), 500)
					}
				}
			}
			if !errors.Is(err, storage.ErrPageAuth) {
				t.Fatalf("tampered %s page: want ErrPageAuth, got %v", tc.name, err)
			}
		})
	}
}

func TestSwappedPagesAreDetected(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 300)
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(env.db)
	if err != nil {
		t.Fatal(err)
	}
	const ps = storage.PageSize
	p2, p3 := append([]byte(nil), b[2*ps:3*ps]...), append([]byte(nil), b[3*ps:4*ps]...)
	copy(b[2*ps:], p3)
	copy(b[3*ps:], p2)
	if err := os.WriteFile(env.db, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := storage.AuthenticateFile(env.db, mustPageCipher(t, env)); !errors.Is(err, storage.ErrPageAuth) {
		t.Fatalf("swapped pages: want ErrPageAuth, got %v", err)
	}
}

func TestZeroedPageInsideFileIsDetected(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 300)
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(env.db, os.O_RDWR, 0)
	_, _ = f.WriteAt(make([]byte, storage.PageSize), 2*storage.PageSize)
	_ = f.Close()
	if _, _, err := storage.AuthenticateFile(env.db, mustPageCipher(t, env)); !errors.Is(err, storage.ErrPageAuth) {
		t.Fatalf("zeroed page: want ErrPageAuth, got %v", err)
	}
	// Truncating the file mid-page is also caught.
	_ = os.Truncate(env.db, 3*storage.PageSize+100)
	if _, _, err := storage.AuthenticateFile(env.db, mustPageCipher(t, env)); !errors.Is(err, storage.ErrPageAuth) {
		t.Fatalf("truncated file: want ErrPageAuth, got %v", err)
	}
}

func mustPageCipher(t *testing.T, env encEnv) *storage.PageCipher {
	t.Helper()
	master, err := crypt.ReadMasterKey(env.key)
	if err != nil {
		t.Fatal(err)
	}
	k, err := crypt.OpenKeyring(env.db, master)
	if err != nil {
		t.Fatal(err)
	}
	return storage.NewPageCipher(k.PageAEAD(), k.ID)
}

func TestTamperedWALMidLogIsDetected(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 30)
	crashed := crashCopy(t, env) // un-checkpointed: the WAL holds every record
	_ = eng.Close()
	info, err := os.Stat(crashed.wal)
	if err != nil {
		t.Fatal(err)
	}
	flipByte(t, crashed.wal, info.Size()/3) // a ciphertext byte of an early frame
	_, err = NewEngineWithOptions(crashed.db, crashed.wal, 32, Options{KeyFile: crashed.key})
	if !errors.Is(err, wal.ErrWALTampered) {
		t.Fatalf("tampered WAL: want ErrWALTampered, got %v", err)
	}
}

func TestWALFormatMixIsRefused(t *testing.T) {
	env := newEncEnv(t)
	eng := env.open(t, 32)
	fillEngine(t, eng, 5)
	crashed := crashCopy(t, env)
	_ = eng.Close()
	// Encrypted WAL under a plaintext reader.
	if _, err := wal.OpenWAL(crashed.wal, false); !errors.Is(err, wal.ErrWALFormat) {
		t.Fatalf("plaintext open of encrypted WAL: want ErrWALFormat, got %v", err)
	}
	// Plaintext WAL under an encrypted engine.
	dir := t.TempDir()
	pw, err := wal.OpenWAL(filepath.Join(dir, "p.wal"), false)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = pw.Write(wal.OpPut, make([]byte, 8), []byte("v"))
	_ = pw.Close()
	k := mustKeys(t, env)
	if _, err := wal.OpenWALCipher(filepath.Join(dir, "p.wal"), false, wal.NewCipher(k.WALAEAD(), k.ID)); !errors.Is(err, wal.ErrWALFormat) {
		t.Fatalf("encrypted open of plaintext WAL: want ErrWALFormat, got %v", err)
	}
}

func mustKeys(t *testing.T, env encEnv) *crypt.Keys {
	t.Helper()
	master, _ := crypt.ReadMasterKey(env.key)
	k, err := crypt.OpenKeyring(env.db, master)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestMigrateEncryptDecryptRekey(t *testing.T) {
	dir := t.TempDir()
	db, w := filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal")
	eng, err := NewEngine(db, w, 32)
	if err != nil {
		t.Fatal(err)
	}
	fillEngine(t, eng, 700)
	big := bytes.Repeat([]byte("L"), 300_000)
	_ = eng.Put(9000, big)
	_ = eng.Close()
	if !fileHasMarker(t, db) {
		t.Fatal("test setup: plaintext database should contain the marker")
	}
	k1, k2 := keyFile(t, dir, "k1"), keyFile(t, dir, "k2")

	res, err := MigrateEncryption(db, w, MigrateOptions{DstKeyFile: k1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 701 || len(res.Kept) == 0 {
		t.Fatalf("encrypt result %+v", res)
	}
	if fileHasMarker(t, db) || fileHasMarker(t, w) {
		t.Fatal("encrypted database still holds plaintext")
	}
	if !fileHasMarker(t, db+".preenc.bak") {
		t.Fatal("original should have been kept as .preenc.bak")
	}
	enc, err := NewEngineWithOptions(db, w, 32, Options{KeyFile: k1})
	if err != nil {
		t.Fatal(err)
	}
	checkEngine(t, enc, 700)
	if got, ok, _ := enc.Get(9000); !ok || !bytes.Equal(got, big) {
		t.Fatal("large value lost in encrypt")
	}
	// New writes after migration work and survive a restart.
	_ = enc.Put(701, value(701))
	_ = enc.Close()

	// Rekey: new master key, new data key, key id advances.
	if _, err := MigrateEncryption(db, w, MigrateOptions{SrcKeyFile: k1, DstKeyFile: k2, DeleteOriginal: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngineWithOptions(db, w, 32, Options{KeyFile: k1}); !errors.Is(err, crypt.ErrWrongKey) {
		t.Fatalf("old key after rekey: want ErrWrongKey, got %v", err)
	}
	rk, err := NewEngineWithOptions(db, w, 32, Options{KeyFile: k2})
	if err != nil {
		t.Fatal(err)
	}
	if rk.Encryption().KeyID != 2 {
		t.Fatalf("key id after rekey = %d", rk.Encryption().KeyID)
	}
	checkEngine(t, rk, 701)
	_ = rk.Close()
	if _, err := os.Stat(db + ".prekey.bak"); err == nil {
		t.Fatal("--delete-original should not leave a .prekey.bak")
	}

	// Decrypt back to plaintext.
	if _, err := MigrateEncryption(db, w, MigrateOptions{SrcKeyFile: k2}); err != nil {
		t.Fatal(err)
	}
	if crypt.HasKeyring(db) {
		t.Fatal("keyring should be moved aside after decrypt")
	}
	pe, err := NewEngine(db, w, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer pe.Close()
	checkEngine(t, pe, 701)
	if pe.Encryption().Enabled {
		t.Fatal("decrypted database reports encryption")
	}
}

func TestMigrateRefusesWithoutKeyOrWithWrongSource(t *testing.T) {
	dir := t.TempDir()
	db, w := filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal")
	if _, err := MigrateEncryption(db, w, MigrateOptions{}); err == nil {
		t.Fatal("no key files must be rejected")
	}
	k1 := keyFile(t, dir, "k1")
	eng, _ := NewEngine(db, w, 32)
	fillEngine(t, eng, 10)
	_ = eng.Close()
	if _, err := MigrateEncryption(db, w, MigrateOptions{DstKeyFile: k1}); err != nil {
		t.Fatal(err)
	}
	// Encrypting an already encrypted database (no source key) must refuse and leave it intact.
	if _, err := MigrateEncryption(db, w, MigrateOptions{DstKeyFile: keyFile(t, dir, "k2")}); !errors.Is(err, storage.ErrEncryptedNoKey) {
		t.Fatalf("want ErrEncryptedNoKey, got %v", err)
	}
	e, err := NewEngineWithOptions(db, w, 32, Options{KeyFile: k1})
	if err != nil {
		t.Fatal(err)
	}
	checkEngine(t, e, 10)
	_ = e.Close()
}
