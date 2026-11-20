package crypt

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testKey(b byte) ([]byte, string) {
	k := bytes.Repeat([]byte{b}, KeySize)
	return k, hex.EncodeToString(k)
}

func TestKeySourceEnv(t *testing.T) {
	want, h := testKey(7)
	t.Setenv("BEASTDB_TEST_KEY", h)
	got, err := ReadMasterKey("env:BEASTDB_TEST_KEY")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("env key: %v", err)
	}
	t.Setenv("BEASTDB_TEST_KEY", "")
	if _, err := ReadMasterKey("env:BEASTDB_TEST_KEY"); !errors.Is(err, ErrKeyEnvUnset) {
		t.Fatalf("want ErrKeyEnvUnset, got %v", err)
	}
	t.Setenv("BEASTDB_TEST_KEY", "not a key")
	_, err = ReadMasterKey("env:BEASTDB_TEST_KEY")
	if !errors.Is(err, ErrBadKeyFile) || strings.Contains(err.Error(), "not a key") {
		t.Fatalf("bad env key must fail without echoing the value: %v", err)
	}
}

func TestKeySourceFileForms(t *testing.T) {
	want, h := testKey(9)
	p := filepath.Join(t.TempDir(), "k")
	if err := os.WriteFile(p, []byte(h+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{p, "file:" + p} {
		if got, err := ReadMasterKey(spec); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %v", spec, err)
		}
	}
	if SourceKind(p) != "file" || SourceKind("env:X") != "env" || SourceKind("exec:x") != "exec" {
		t.Fatal("SourceKind")
	}
}

// helperKeyCmd is an exec source that re-runs the test binary as a tiny program
// (no shell, no OS-specific tools).
func helperKeyCmd(mode string) string {
	exe, _ := os.Executable()
	return `exec:["` + strings.ReplaceAll(exe, `\`, `\\`) + `","-test.run=TestHelperKeyProcess","--","` + mode + `"]`
}

func TestHelperKeyProcess(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		t.Skip("helper process")
	}
	switch os.Args[len(os.Args)-1] {
	case "ok":
		_, h := testKey(5)
		os.Stdout.WriteString(h + "\n")
	case "fail":
		os.Stderr.WriteString("SECRET-IN-STDERR " + strings.Repeat("a", 64))
		os.Exit(3)
	case "big":
		os.Stdout.WriteString(strings.Repeat("a", 100000))
	case "short":
		os.Stdout.WriteString("abcd")
	}
	os.Exit(0)
}

func TestKeySourceExec(t *testing.T) {
	want, _ := testKey(5)
	got, err := ReadMasterKey(helperKeyCmd("ok"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("exec key: %v", err)
	}
	_, err = ReadMasterKey(helperKeyCmd("fail"))
	if !errors.Is(err, ErrKeyCommand) || strings.Contains(err.Error(), "SECRET-IN-STDERR") {
		t.Fatalf("failing command: want ErrKeyCommand without stderr, got %v", err)
	}
	if _, err := ReadMasterKey(helperKeyCmd("big")); !errors.Is(err, ErrKeyCommand) {
		t.Fatalf("oversized output: %v", err)
	}
	if _, err := ReadMasterKey(helperKeyCmd("short")); !errors.Is(err, ErrBadKeyFile) {
		t.Fatalf("short output: %v", err)
	}
	if _, err := ReadMasterKey("exec:/definitely/not/a/program"); !errors.Is(err, ErrKeyCommand) {
		t.Fatalf("missing program: %v", err)
	}
	if _, err := ReadMasterKey("exec:"); err == nil {
		t.Fatal("empty command must fail")
	}
	if runtime.GOOS != "windows" {
		// No shell: metacharacters are plain arguments, never interpreted.
		if _, err := exec.LookPath("echo"); err == nil {
			marker := filepath.Join(t.TempDir(), "marker")
			if _, err := ReadMasterKey("exec:echo hi; touch " + marker); err == nil {
				t.Fatal("echo output is not a key")
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("command line was interpreted by a shell")
			}
		}
	}
}

func TestSpecFromParts(t *testing.T) {
	if s, err := SpecFromParts("", "", ""); s != "" || err != nil {
		t.Fatal(s, err)
	}
	if s, _ := SpecFromParts("", "K", ""); s != "env:K" {
		t.Fatal(s)
	}
	if s, _ := SpecFromParts("", "", "kms get"); s != "exec:kms get" {
		t.Fatal(s)
	}
	if _, err := SpecFromParts("f", "K", ""); err == nil {
		t.Fatal("two sources must be rejected")
	}
}

func TestZero(t *testing.T) {
	b := []byte{1, 2, 3}
	Zero(b)
	if !bytes.Equal(b, []byte{0, 0, 0}) {
		t.Fatal("not zeroed")
	}
}

func TestRewrapKeepsDEKAndIsCrashSafe(t *testing.T) {
	db := filepath.Join(t.TempDir(), "beast.bin")
	oldM, _ := testKey(1)
	newM, _ := testKey(2)
	k, err := CreateKeyring(db, oldM, 4)
	if err != nil {
		t.Fatal(err)
	}
	n := make([]byte, 12)
	ct := k.PageAEAD().Seal(nil, n, []byte("page"), nil)

	// A crash after the temp file was written but before the rename leaves a
	// stray .tmp: the old key must still work and a later Rewrap must succeed.
	before, _ := os.ReadFile(KeyringPath(db))
	if err := os.WriteFile(KeyringPath(db)+".tmp", []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenKeyring(db, oldM); err != nil {
		t.Fatalf("old key must work before the swap: %v", err)
	}
	if err := Rewrap(db, newM, oldM); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("wrong current key must be refused, got %v", err)
	}
	if after, _ := os.ReadFile(KeyringPath(db)); !bytes.Equal(before, after) {
		t.Fatal("failed rewrap changed the keyring")
	}

	if err := Rewrap(db, oldM, newM); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(KeyringPath(db) + ".tmp"); err == nil {
		t.Fatal("temp file left behind")
	}
	if _, err := OpenKeyring(db, oldM); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("old key must stop working: %v", err)
	}
	k2, err := OpenKeyring(db, newM)
	if err != nil || k2.ID != 4 {
		t.Fatalf("new key: %v", err)
	}
	if _, err := k2.PageAEAD().Open(nil, n, ct, nil); err != nil {
		t.Fatalf("DEK changed by rewrap: %v", err)
	}
	if fi, _ := os.Stat(KeyringPath(db)); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("keyring mode %v", fi.Mode().Perm())
	}
}
