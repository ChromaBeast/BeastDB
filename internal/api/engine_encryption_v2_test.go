package api

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/crypt"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// Format version 2 (page reuse, saved free-space map and search index, which
// live in chain pages) works on an encrypted database, and page 0 records both
// the version and the encrypted flag.
func TestEncryptedFormatV2PageReuseAndSavedMaps(t *testing.T) {
	env := newEncEnv(t)
	e := env.open(t, 64)
	val := func(i uint64) string {
		return fmt.Sprintf("%s-%04d-%s", secretMarker, i, bytes.Repeat([]byte("z"), 40))
	}
	applyRange(t, e, 1, 3000, 1, val)
	grown := e.disk.NumPages()
	for round := 0; round < 3; round++ { // churn: freed pages must be reused
		applyRange(t, e, 1, 3000, 1, nil)
		applyRange(t, e, 1, 3000, 1, val)
	}
	if after := e.disk.NumPages(); after > grown+grown/10 {
		t.Fatalf("churn grew an encrypted file from %d to %d pages", grown, after)
	}
	applyRange(t, e, 1, 3000, 3, nil) // leave pages partly used: the map has entries
	tracked := len(e.reclaim)
	if tracked == 0 {
		t.Fatal("no pages tracked as having free space")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if fileHasMarker(t, env.db) {
		t.Fatal("plaintext in the encrypted data file")
	}

	meta, pages, err := storage.AuthenticateFile(env.db, mustPageCipher(t, env))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Version != storage.MetaVersion || meta.Flags&storage.MetaFlagEncrypted == 0 || pages == 0 {
		t.Fatalf("meta %+v pages %d", meta, pages)
	}
	if meta.SpaceMapHead == 0 || meta.IndexSnapHead == 0 {
		t.Fatalf("maps not saved through chain pages: %+v", meta)
	}

	re := env.open(t, 64)
	defer re.Close()
	if len(re.reclaim) != tracked {
		t.Fatalf("free-space map after restart: %d pages, want %d", len(re.reclaim), tracked)
	}
	for k := uint64(1); k <= 3000; k++ {
		got, ok, err := re.Get(k)
		if err != nil || ok != (k%3 != 1) || (ok && string(got) != val(k)) {
			t.Fatalf("key %d after restart: ok=%v err=%v", k, ok, err)
		}
	}
	before := re.disk.NumPages()
	applyRange(t, re, 1, 3000, 3, val)
	if after := re.disk.NumPages(); after > before+2 {
		t.Fatalf("re-inserting freed records grew the file from %d to %d pages", before, after)
	}
}

// A format version 1 plaintext database: opening it with a key is refused and
// leaves it untouched; encrypting it migrates it to version 2 first (a version
// 1 file cannot be encrypted page by page), applying the WAL tail, and the
// result is a version 2 encrypted database. Plaintext open still migrates it.
func TestV1MigrationWithAndWithoutEncryption(t *testing.T) {
	check := func(t *testing.T, e *Engine) {
		t.Helper()
		for i := uint64(1); i <= 50; i++ {
			v, ok, err := e.Get(i)
			if i == 7 {
				if ok {
					t.Fatal("delete from the WAL tail was not applied")
				}
				continue
			}
			if err != nil || !ok || string(v) != fmt.Sprintf("v1-value-%d", i) {
				t.Fatalf("key %d: %q ok=%v err=%v", i, v, ok, err)
			}
		}
		if v, _, err := e.Get(99); err != nil || len(v) != 3*MaxRecordSize {
			t.Fatalf("large value: %d bytes, err=%v", len(v), err)
		}
		if v, _, _ := e.Get(500); string(v) != "from-wal-tail" {
			t.Fatalf("put from the WAL tail = %q", v)
		}
	}

	t.Run("key on a v1 file is refused and the file is untouched", func(t *testing.T) {
		dir := t.TempDir()
		db, w := createV1Database(t, dir)
		before, _ := os.ReadFile(db)
		key := keyFile(t, dir, "k")
		if _, err := NewEngineWithOptions(db, w, 64, Options{KeyFile: key}); !errors.Is(err, storage.ErrNotEncrypted) {
			t.Fatalf("want ErrNotEncrypted, got %v", err)
		}
		after, _ := os.ReadFile(db)
		if !bytes.Equal(before, after) || crypt.HasKeyring(db) {
			t.Fatal("a refused open changed the v1 database or created a keyring")
		}
		e, err := NewEngine(db, w, 64) // plaintext open still migrates v1 -> v2
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		check(t, e)
	})

	t.Run("encrypt migrates v1 through v2", func(t *testing.T) {
		dir := t.TempDir()
		db, w := createV1Database(t, dir)
		key := keyFile(t, dir, "k")
		res, err := MigrateEncryption(db, w, MigrateOptions{DstKeyFile: key})
		if err != nil {
			t.Fatal(err)
		}
		if res.Records != 51 { // 49 left of 1..50, plus 99 and 500
			t.Fatalf("records copied: %d", res.Records)
		}
		e, err := NewEngineWithOptions(db, w, 64, Options{KeyFile: key})
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		check(t, e)
		if !e.Encryption().Enabled {
			t.Fatal("not encrypted after migration")
		}
		meta, _, err := storage.AuthenticateFile(db, mustPageCipherFor(t, db, key))
		if err != nil || meta.Version != storage.MetaVersion || meta.Flags&storage.MetaFlagEncrypted == 0 {
			t.Fatalf("meta %+v err %v", meta, err)
		}
	})
}

func mustPageCipherFor(t *testing.T, db, keyPath string) *storage.PageCipher {
	t.Helper()
	master, err := crypt.ReadMasterKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	k, err := crypt.OpenKeyring(db, master)
	if err != nil {
		t.Fatal(err)
	}
	return storage.NewPageCipher(k.PageAEAD(), k.ID)
}

// An encrypted log with a torn tail is cut at open, so the next writes are not
// appended behind it and survive another crash.
func TestEncryptedWriteAfterTornTailSurvivesNextCrash(t *testing.T) {
	env := newEncEnv(t)
	e := env.open(t, 32)
	fillEngine(t, e, 20)
	crashed := encCrashCopy(t, env)
	_ = e.Close()
	f, err := os.OpenFile(crashed.wal, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(bytes.Repeat([]byte{0x7}, 40)) // half a frame
	_ = f.Close()

	e2 := crashed.open(t, 32)
	if err := e2.Put(500, value(500)); err != nil {
		t.Fatal(err)
	}
	crashed2 := encCrashCopy(t, crashed)
	_ = e2.Close()
	e3 := crashed2.open(t, 32)
	defer e3.Close()
	checkEngine(t, e3, 20)
	if got, ok, _ := e3.Get(500); !ok || !bytes.Equal(got, value(500)) {
		t.Fatal("write made after a torn tail was lost by the next crash")
	}
}

// Damage in the middle of an encrypted log fails the open with the same
// ErrCorruptLog as a plaintext log, and the truncation option cuts it there,
// saving the cut bytes.
func TestEncryptedMidLogDamageAndTruncateOption(t *testing.T) {
	env := newEncEnv(t)
	e := env.open(t, 32)
	fillEngine(t, e, 30)
	crashed := encCrashCopy(t, env)
	_ = e.Close()
	info, _ := os.Stat(crashed.wal)
	flipByte(t, crashed.wal, info.Size()/2)

	if _, err := NewEngineWithOptions(crashed.db, crashed.wal, 32, Options{KeyFile: crashed.key}); !errors.Is(err, wal.ErrCorruptLog) {
		t.Fatalf("want ErrCorruptLog, got %v", err)
	}
	if _, err := os.Stat(crashed.wal + ".discarded"); err == nil {
		t.Fatal("a refused open must not cut the log")
	}
	e2, err := NewEngineWithOptions(crashed.db, crashed.wal, 32, Options{KeyFile: crashed.key, TruncateCorruptWAL: true})
	if err != nil {
		t.Fatalf("open with truncation: %v", err)
	}
	defer e2.Close()
	if _, err := os.Stat(crashed.wal + ".discarded"); err != nil {
		t.Fatalf("cut bytes not saved: %v", err)
	}
	// Records before the damage are kept; later writes still work and the log reopens.
	kept := 0
	for i := 1; i <= 30; i++ {
		if _, ok, _ := e2.Get(uint64(i)); ok {
			kept++
		}
	}
	if kept == 0 || kept == 30 {
		t.Fatalf("expected a strict prefix of the 30 records, kept %d", kept)
	}
	if err := e2.Put(900, value(900)); err != nil {
		t.Fatal(err)
	}
}

// Rotating the WAL (checkpoint) keeps the encrypted format: the new log has its
// header, new records are readable after a crash, and the archived generation
// is still authenticated.
func TestEncryptedWALRotationAndCrash(t *testing.T) {
	env := newEncEnv(t)
	e := env.open(t, 32)
	fillEngine(t, e, 20)
	if err := e.Checkpoint(); err != nil {
		t.Fatal(err)
	}
	for i := 21; i <= 40; i++ {
		if err := e.Put(uint64(i), value(i)); err != nil {
			t.Fatal(err)
		}
	}
	crashed := encCrashCopy(t, env)
	_ = e.Close()
	r := crashed.open(t, 32)
	defer r.Close()
	checkEngine(t, r, 40)
	if fileHasMarker(t, crashed.wal) || fileHasMarker(t, crashed.wal+".bak") {
		t.Fatal("plaintext in a rotated WAL generation")
	}
}
