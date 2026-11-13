package audit

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestChainAppendReopenAndList(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{ActionLoginOK, ActionTokenCreate, ActionCheckpoint} {
		if err := l.Record("admin", a, "t", "d", "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}
	if st := l.Verify(); !st.Verified || st.Entries != 3 {
		t.Fatalf("verify: %+v", st)
	}
	_ = l.Close()

	// Reopening continues the chain instead of restarting it.
	l, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_ = l.Record("admin", ActionLogout, "", "", "")
	got, _ := l.List(Query{})
	if len(got) != 4 || got[0].Seq != 4 || got[0].Action != ActionLogout || got[3].Seq != 1 {
		t.Fatalf("list newest first: %+v", got)
	}
	if got[0].Prev != got[1].Hash {
		t.Fatal("new record must link to the previous hash")
	}
	if f, _ := l.List(Query{Action: ActionCheckpoint}); len(f) != 1 {
		t.Fatalf("action filter: %+v", f)
	}
	if p, _ := l.List(Query{Before: 3, Limit: 10}); len(p) != 2 || p[0].Seq != 2 {
		t.Fatalf("paging: %+v", p)
	}
}

func TestTamperingIsDetected(t *testing.T) {
	cases := map[string]func(lines []string) []string{
		"edit":    func(l []string) []string { l[1] = strings.Replace(l[1], `"admin"`, `"mallory"`, 1); return l },
		"delete":  func(l []string) []string { return append(l[:1], l[2:]...) },
		"reorder": func(l []string) []string { l[1], l[2] = l[2], l[1]; return l },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			l, _ := Open(dir)
			for i := 0; i < 4; i++ {
				_ = l.Record("admin", ActionCheckpoint, "", "", "")
			}
			_ = l.Close()

			path := filepath.Join(dir, FileName)
			raw, _ := os.ReadFile(path)
			lines := mutate(strings.Split(strings.TrimSpace(string(raw)), "\n"))
			_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)

			if _, err := Open(dir); err == nil {
				t.Fatal("Open must refuse a damaged chain")
			}
		})
	}
}

func TestNilLogAndSanitising(t *testing.T) {
	var l *Log
	if err := l.Record("a", "b", "c", "d", "e"); err != nil {
		t.Fatal("nil log must discard silently")
	}
	if st := l.Verify(); !st.Verified {
		t.Fatal("nil log verifies trivially")
	}

	dir := t.TempDir()
	real, _ := Open(dir)
	defer real.Close()
	_ = real.Record("ad\nmin", "x", strings.Repeat("a", 2000), "line1\r\nline2", "")
	e, _ := real.List(Query{})
	if strings.ContainsAny(e[0].Actor+e[0].Detail, "\r\n") || len(e[0].Target) > maxField+4 {
		t.Fatalf("fields must be single-line and bounded: %+v", e[0])
	}
	if st := real.Verify(); !st.Verified {
		t.Fatalf("sanitised entry must still verify: %+v", st)
	}
}

func TestConcurrentRecordsKeepChainValid(t *testing.T) {
	l, _ := Open(t.TempDir())
	defer l.Close()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_ = l.Record("a", ActionCheckpoint, "", "", "")
			}
		}()
	}
	wg.Wait()
	if st := l.Verify(); !st.Verified || st.Entries != 160 {
		t.Fatalf("%+v", st)
	}
}
