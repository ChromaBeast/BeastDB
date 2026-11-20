package backup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
)

func payload(i uint64) []byte {
	return []byte(fmt.Sprintf("online-%d-%s", i, "pppppppppppppppppppppppppppppppp"))
}

// backupUnderWrites writes sequential keys non-stop while backups are made,
// then verifies every bundle and opens it: each must be a gap-free prefix of
// the key stream, so no backup holds a half-applied or torn state.
func backupUnderWrites(t *testing.T, e *api.Engine, key string) {
	t.Helper()
	var next atomic.Uint64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			k := next.Add(1)
			if e.Put(k, payload(k)) != nil {
				return
			}
		}
	}()
	root := filepath.Join(t.TempDir(), "backups")
	mgr := NewManager(e, root)
	var ids []string
	for next.Load() < 100 {
	}
	for i := 0; i < 3; i++ {
		info, err := mgr.Create(KindManual, "root")
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		ids = append(ids, info.ID)
	}
	close(stop)
	wg.Wait()

	for _, id := range ids {
		dir := filepath.Join(root, id)
		if res, err := mgr.Verify(id); err != nil || !res.OK {
			t.Fatalf("%s verify: %v %+v", id, err, res)
		}
		// Restore as beastctl does: data.bin (and keyring) into a fresh directory.
		rd := t.TempDir()
		b, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
		_ = os.WriteFile(filepath.Join(rd, "beast.bin"), b, 0o600)
		opts := api.Options{}
		if key != "" {
			kr, _ := os.ReadFile(filepath.Join(dir, KeyringName))
			_ = os.WriteFile(crypt.KeyringPath(filepath.Join(rd, "beast.bin")), kr, 0o600)
			opts.KeyFile = key
		}
		r, err := api.NewEngineWithOptions(filepath.Join(rd, "beast.bin"), filepath.Join(rd, "beast.wal"), 32, opts)
		if err != nil {
			t.Fatalf("%s: open restored: %v", id, err)
		}
		n := uint64(0)
		for {
			v, ok, err := r.Get(n + 1)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if !bytes.Equal(v, payload(n+1)) {
				t.Fatalf("%s: key %d corrupt", id, n+1)
			}
			n++
		}
		if n < 100 {
			t.Fatalf("%s: only %d keys", id, n)
		}
		for k := n + 2; k <= next.Load(); k += 5 {
			if _, ok, _ := r.Get(k); ok {
				t.Fatalf("%s: key %d present after gap at %d", id, k, n+1)
			}
		}
		_ = r.Close()
	}
}

func TestBackupDuringWritesVerifiesAndRestores(t *testing.T) {
	dir := t.TempDir()
	e, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	backupUnderWrites(t, e, "")
}

func TestEncryptedBackupDuringWritesVerifiesAndRestores(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "master.key")
	if err := crypt.GenerateKeyFile(key); err != nil {
		t.Fatal(err)
	}
	e, err := api.NewEngineWithOptions(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 16, api.Options{KeyFile: key})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	backupUnderWrites(t, e, key)
}
