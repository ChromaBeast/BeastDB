package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
)

func newEngine(t *testing.T, records int) *api.Engine {
	t.Helper()
	dir := t.TempDir()
	e, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	for i := uint64(1); i <= uint64(records); i++ {
		if err := e.Put(i, []byte("value-"+strings.Repeat("x", 50))); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestCreateVerifyListAndCorruption(t *testing.T) {
	e := newEngine(t, 200)
	root := filepath.Join(t.TempDir(), "backups")

	info, err := Create(e, root, KindManual, "root")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(info.ID) || info.Kind != KindManual || info.CreatedBy != "root" || info.FileSize == 0 || info.RootPageID == 0 {
		t.Fatalf("%+v", info)
	}
	if _, err := Verify(filepath.Join(root, info.ID)); err != nil {
		t.Fatalf("fresh backup must verify: %v", err)
	}
	for _, f := range []string{"data.bin", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(root, info.ID, f)); err != nil {
			t.Fatalf("bundle is missing %s", f)
		}
	}

	second, _ := Create(e, root, KindScheduled, "scheduler")
	list, _ := List(root)
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != info.ID {
		t.Fatalf("newest first: %+v", list)
	}

	// Flip a byte: the checksum must catch it.
	p := filepath.Join(root, info.ID, "data.bin")
	b, _ := os.ReadFile(p)
	b[len(b)-1] ^= 0xFF
	_ = os.WriteFile(p, b, 0o600)
	if _, err := Verify(filepath.Join(root, info.ID)); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corruption not detected: %v", err)
	}
}

func TestValidIDRejectsPathTricks(t *testing.T) {
	for _, bad := range []string{"", "..", "../etc", "bk-1", "bk-20260101-000000/../x", "bk-20260101-000000-", "BK-20260101-000000", "/abs"} {
		if ValidID(bad) {
			t.Fatalf("%q must not be a valid id", bad)
		}
	}
	for _, ok := range []string{"bk-20260101-000000", "bk-20260101-000000-2"} {
		if !ValidID(ok) {
			t.Fatalf("%q should be valid", ok)
		}
	}
}

// fake blocks in CreateSnapshot so concurrency and retention can be tested without a real engine.
type fake struct {
	gate  chan struct{}
	enter chan struct{}
	size  uint64
}

func (f *fake) CreateSnapshot(dir string) (string, error) {
	if f.enter != nil {
		f.enter <- struct{}{}
		<-f.gate
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "snap.bin")
	return p, os.WriteFile(p, make([]byte, 8192), 0o600)
}
func (f *fake) StorageMetrics() api.StorageMetrics { return api.StorageMetrics{DiskSizeBytes: f.size} }

func TestManagerSingleFlight(t *testing.T) {
	f := &fake{gate: make(chan struct{}), enter: make(chan struct{}, 1)}
	m := NewManager(f, filepath.Join(t.TempDir(), "b"))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _, _ = m.Create(KindManual, "a") }()
	<-f.enter // first backup is now inside CreateSnapshot
	if _, err := m.Create(KindManual, "b"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second concurrent backup must be refused: %v", err)
	}
	close(f.gate)
	wg.Wait()
}

func TestPruneKeepsManualAndNewestScheduled(t *testing.T) {
	f := &fake{}
	root := filepath.Join(t.TempDir(), "b")
	m := NewManager(f, root)
	m.Configure(0, 2)

	var scheduled []string
	for i := 0; i < 4; i++ {
		in, err := m.Create(KindScheduled, "s")
		if err != nil {
			t.Fatal(err)
		}
		scheduled = append(scheduled, in.ID)
		time.Sleep(1100 * time.Millisecond) // ids have one-second resolution
	}
	manual, _ := m.Create(KindManual, "root")
	m.prune()

	got, _ := m.List()
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	if len(got) != 3 || !ids[manual.ID] || !ids[scheduled[3]] || !ids[scheduled[2]] || ids[scheduled[0]] {
		t.Fatalf("expected manual + 2 newest scheduled, got %+v", got)
	}
}

func TestNoSpaceIsRefusedBeforeCopying(t *testing.T) {
	f := &fake{size: 1 << 62}
	_, err := Create(f, filepath.Join(t.TempDir(), "b"), KindManual, "x")
	if !errors.Is(err, ErrNoSpace) {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyUnknownID(t *testing.T) {
	m := NewManager(&fake{}, t.TempDir())
	for _, id := range []string{"../x", "bk-20260101-000000"} {
		if _, err := m.Verify(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q: %v", id, err)
		}
	}
}
