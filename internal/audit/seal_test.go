package audit

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCipher(t *testing.T, fill byte, id uint16) *Cipher {
	t.Helper()
	b, err := aes.NewCipher(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		t.Fatal(err)
	}
	return NewCipher(g, id)
}

func fill(t *testing.T, l *Log, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := l.Record("admin", ActionLoginOK, "target-secret-name", "detail-secret", "10.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSealedLogHidesContentAndVerifies(t *testing.T) {
	dir := t.TempDir()
	c := testCipher(t, 1, 1)
	l, err := OpenSealed(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	fill(t, l, 5)
	if st := l.Verify(); !st.Verified || st.Entries != 5 || !st.Encrypted {
		t.Fatalf("status %+v", st)
	}
	_ = l.Close()
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	for _, leak := range []string{"target-secret-name", "detail-secret", "login.ok", "10.0.0.1", "admin"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("sealed log leaks %q", leak)
		}
	}
	l2, err := OpenSealed(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	fill(t, l2, 2)
	got, err := l2.List(Query{})
	if err != nil || len(got) != 7 || got[0].Seq != 7 || got[0].Target != "target-secret-name" {
		t.Fatalf("list: %v %d", err, len(got))
	}
}

func TestSealedLogRefusesWrongKeyNoKeyAndTampering(t *testing.T) {
	dir := t.TempDir()
	c := testCipher(t, 1, 1)
	l, _ := OpenSealed(dir, c)
	fill(t, l, 4)
	_ = l.Close()
	path := filepath.Join(dir, FileName)

	if _, err := Open(dir); !errors.Is(err, ErrSealed) {
		t.Fatalf("no key: want ErrSealed, got %v", err)
	}
	if _, err := OpenSealed(dir, testCipher(t, 2, 1)); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong key: want ErrAuth, got %v", err)
	}

	orig, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(orig)), "\n")
	write := func(ls ...string) { _ = os.WriteFile(path, []byte(strings.Join(ls, "\n")+"\n"), 0o600) }

	write(lines[0], lines[2], lines[1], lines[3]) // reorder: AAD binds the position
	if _, err := OpenSealed(dir, c); !errors.Is(err, ErrAuth) {
		t.Fatalf("reordered: %v", err)
	}
	write(lines[0], lines[1], lines[3]) // remove a line: the later line moves position
	if _, err := OpenSealed(dir, c); err == nil {
		t.Fatal("removed line accepted")
	}
	bad := []byte(lines[1])
	bad[len(bad)-3] ^= 1
	write(lines[0], string(bad), lines[2], lines[3]) // bit flip
	if _, err := OpenSealed(dir, c); err == nil {
		t.Fatal("flipped bit accepted")
	}
	write(append(lines, `{"seq":5,"time":"2026-01-01T00:00:00Z","actor":"x","action":"login.ok","prev":"","hash":""}`)...)
	if _, err := OpenSealed(dir, c); !errors.Is(err, ErrMixedFormat) {
		t.Fatalf("injected plaintext line: want ErrMixedFormat, got %v", err)
	}
	write(lines...) // intact again
	if l, err := OpenSealed(dir, c); err != nil {
		t.Fatal(err)
	} else {
		_ = l.Close()
	}
}

func TestPlaintextLogStillOpensAndMigratesBothWays(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	fill(t, l, 6)
	chain, _ := l.List(Query{})
	_ = l.Close()
	c := testCipher(t, 3, 2)

	// Plaintext -> sealed on open; chain (hashes) unchanged.
	sl, err := OpenSealed(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	if st := sl.Verify(); !st.Verified || st.Entries != 6 || !st.Encrypted {
		t.Fatalf("after migration: %+v", st)
	}
	after, _ := sl.List(Query{})
	for i := range chain {
		if chain[i].Hash != after[i].Hash {
			t.Fatal("hash chain changed by migration")
		}
	}
	fill(t, sl, 1)
	_ = sl.Close()
	if _, err := os.Stat(filepath.Join(dir, FileName+MigrationSuffix)); err == nil {
		t.Fatal("temp file left behind")
	}

	// Sealed -> plaintext through the explicit conversion used by decrypt.
	if ok, err := PrepareConvert(dir, c, nil); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := InstallConverted(dir); err != nil {
		t.Fatal(err)
	}
	pl, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if st := pl.Verify(); !st.Verified || st.Entries != 7 || st.Encrypted {
		t.Fatalf("after decrypt: %+v", st)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	if !strings.Contains(string(raw), "login.ok") {
		t.Fatal("plaintext log expected")
	}
}

func TestInterruptedConversionIsFinishedAtOpen(t *testing.T) {
	dir := t.TempDir()
	old, nw := testCipher(t, 1, 1), testCipher(t, 9, 2)
	l, _ := OpenSealed(dir, old)
	fill(t, l, 3)
	_ = l.Close()
	// A rekey prepared the file under the new keys, swapped the database, and
	// crashed before installing it: audit.log is still sealed under the old key.
	if _, err := PrepareConvert(dir, old, nw); err != nil {
		t.Fatal(err)
	}
	l2, err := OpenSealed(dir, nw)
	if err != nil {
		t.Fatalf("open under the new key should finish the conversion: %v", err)
	}
	if st := l2.Verify(); !st.Verified || st.Entries != 3 {
		t.Fatalf("%+v", st)
	}
	_ = l2.Close()

	// A crash before the database swap: the engine still uses the old key and
	// the prepared file (new key) is discarded.
	dir2 := t.TempDir()
	l3, _ := OpenSealed(dir2, old)
	fill(t, l3, 2)
	_ = l3.Close()
	_, _ = PrepareConvert(dir2, old, nw)
	l4, err := OpenSealed(dir2, old)
	if err != nil {
		t.Fatal(err)
	}
	_ = l4.Close()
	if _, err := os.Stat(filepath.Join(dir2, FileName+MigrationSuffix)); err == nil {
		t.Fatal("stale prepared file kept")
	}
}
