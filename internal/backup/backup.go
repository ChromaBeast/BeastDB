// Package backup creates, verifies and lists physical backups of a running
// engine. The on-disk format is the one `beastctl backup` writes (data.bin plus
// manifest.json), so `beastctl verify` and `beastctl restore` accept bundles
// made here. Restoring stays an offline `beastctl restore` step.
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Manifest is manifest.json. The first block is shared with beastctl; Kind and
// CreatedBy are optional extras that beastctl ignores.
type Manifest struct {
	Version            string `json:"version"`
	CreatedAt          string `json:"created_at"`
	DataChecksumSHA256 string `json:"data_checksum_sha256"`
	RootPageID         uint64 `json:"root_page_id"`
	ActiveDataPageID   uint64 `json:"active_data_page_id"`
	LastCheckpointLSN  uint64 `json:"last_checkpoint_lsn"`
	FileSize           int64  `json:"file_size"`

	Kind      string `json:"kind,omitempty"` // "manual" or "scheduled"
	CreatedBy string `json:"created_by,omitempty"`
}

// Info is a listed backup.
type Info struct {
	ID string `json:"id"`
	Manifest
}

// Snapshotter is the slice of the engine a backup needs.
type Snapshotter interface {
	// CreateSnapshot flushes dirty pages and copies the data file into dir while
	// holding the engine lock, so the copy is consistent. Reads and writes wait
	// for the copy (time grows with the data file size).
	CreateSnapshot(dir string) (string, error)
	StorageMetrics() api.StorageMetrics
}

// Kinds of backup.
const (
	KindManual    = "manual"
	KindScheduled = "scheduled"
)

var idPattern = regexp.MustCompile(`^bk-\d{8}-\d{6}(-\d+)?$`)

// ValidID reports whether id has the shape this package generates. Handlers
// must check it before using an id in a path.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// ErrNoSpace is returned when the backup directory lacks room for the copy.
var ErrNoSpace = errors.New("not enough free disk space for a backup")

func newID(root string) string {
	base := "bk-" + time.Now().UTC().Format("20060102-150405")
	id := base
	for n := 1; ; n++ {
		if _, err := os.Stat(filepath.Join(root, id)); os.IsNotExist(err) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// Create writes a new backup bundle under root/<id>/ and returns it. On any
// failure the partial directory is removed.
func Create(eng Snapshotter, root, kind, createdBy string) (Info, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return Info{}, err
	}
	if need := eng.StorageMetrics().DiskSizeBytes; need > 0 {
		if free, ok := metrics.DiskFreeBytes(root); ok && free < need+need/10 {
			return Info{}, fmt.Errorf("%w: need about %d bytes, %d free", ErrNoSpace, need+need/10, free)
		}
	}
	id := newID(root)
	dir := filepath.Join(root, id)
	fail := func(err error) (Info, error) { _ = os.RemoveAll(dir); return Info{}, err }

	snap, err := eng.CreateSnapshot(dir)
	if err != nil {
		return fail(fmt.Errorf("snapshot: %w", err))
	}
	data := filepath.Join(dir, "data.bin")
	if err := os.Rename(snap, data); err != nil {
		return fail(err)
	}
	sum, size, err := checksum(data)
	if err != nil {
		return fail(err)
	}
	m := Manifest{
		Version: "1.0.0", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		DataChecksumSHA256: sum, FileSize: size, Kind: kind, CreatedBy: createdBy,
	}
	if meta, err := readMeta(data); err == nil {
		m.RootPageID, m.ActiveDataPageID, m.LastCheckpointLSN = meta.RootPageID, meta.ActiveDataPageID, meta.LastCheckpointLSN
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o640); err != nil {
		return fail(err)
	}
	return Info{ID: id, Manifest: m}, nil
}

func checksum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func readMeta(path string) (storage.MetaData, error) {
	f, err := os.Open(path)
	if err != nil {
		return storage.MetaData{}, err
	}
	defer f.Close()
	page := make([]byte, storage.PageSize)
	if _, err := f.ReadAt(page, 0); err != nil {
		return storage.MetaData{}, err
	}
	return storage.DecodeMeta(page)
}

// Verify re-hashes data.bin and checks it against manifest.json, the same
// checks `beastctl verify` makes.
func Verify(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("missing or unreadable manifest.json: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	sum, size, err := checksum(filepath.Join(dir, "data.bin"))
	if err != nil {
		return m, fmt.Errorf("missing or unreadable data.bin: %w", err)
	}
	if sum != m.DataChecksumSHA256 {
		return m, fmt.Errorf("checksum mismatch: expected %s, computed %s", m.DataChecksumSHA256, sum)
	}
	if size != m.FileSize {
		return m, fmt.Errorf("file size mismatch: manifest says %d bytes, found %d", m.FileSize, size)
	}
	meta, err := readMeta(filepath.Join(dir, "data.bin"))
	if err != nil {
		return m, fmt.Errorf("page 0 metadata validation failed: %w", err)
	}
	if meta.RootPageID != m.RootPageID {
		return m, fmt.Errorf("root page mismatch: manifest=%d, page0=%d", m.RootPageID, meta.RootPageID)
	}
	return m, nil
}

// List returns the backups under root, newest first. Directories without a
// readable manifest are skipped.
func List(root string) ([]Info, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	out := []Info{}
	for _, e := range ents {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		out = append(out, Info{ID: e.Name(), Manifest: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
