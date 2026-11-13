package backup

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrBusy is returned when a backup is already running.
var ErrBusy = errors.New("a backup is already running")

// ErrNotFound is returned for an unknown or malformed backup id.
var ErrNotFound = errors.New("backup not found")

// Result is the last verification outcome for one backup, kept in memory.
type Result struct {
	OK    bool      `json:"ok"`
	At    time.Time `json:"at"`
	Error string    `json:"error,omitempty"`
}

// Status is a listed backup plus its last known verification.
type Status struct {
	Info
	Verified *Result `json:"verified"` // nil until verified since this process started
}

// Manager owns the backup directory, serialises backups and runs the optional
// schedule.
type Manager struct {
	root string
	eng  Snapshotter

	run     sync.Mutex // held while a backup is being created
	mu      sync.Mutex // guards results and schedule fields
	results map[string]Result
	every   time.Duration
	keep    int
	wake    chan struct{}
}

// NewManager returns a Manager storing backups under root.
func NewManager(eng Snapshotter, root string) *Manager {
	return &Manager{root: root, eng: eng, results: map[string]Result{}, keep: 7, wake: make(chan struct{}, 1)}
}

// Dir is the backup directory.
func (m *Manager) Dir() string { return m.root }

// Create makes a backup now and verifies it straight away.
func (m *Manager) Create(kind, actor string) (Info, error) {
	if !m.run.TryLock() {
		return Info{}, ErrBusy
	}
	defer m.run.Unlock()
	info, err := Create(m.eng, m.root, kind, actor)
	if err != nil {
		return Info{}, err
	}
	m.record(info.ID)
	return info, nil
}

func (m *Manager) record(id string) Result {
	r := Result{OK: true, At: time.Now().UTC()}
	if _, err := Verify(filepath.Join(m.root, id)); err != nil {
		r.OK, r.Error = false, err.Error()
	}
	m.mu.Lock()
	m.results[id] = r
	m.mu.Unlock()
	return r
}

// Verify re-checks one backup against its manifest.
func (m *Manager) Verify(id string) (Result, error) {
	if !ValidID(id) {
		return Result{}, ErrNotFound
	}
	if _, err := os.Stat(filepath.Join(m.root, id, "manifest.json")); err != nil {
		return Result{}, ErrNotFound
	}
	return m.record(id), nil
}

// Path returns the directory of a backup after validating its id.
func (m *Manager) Path(id string) (string, error) {
	if !ValidID(id) {
		return "", ErrNotFound
	}
	dir := filepath.Join(m.root, id)
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return "", ErrNotFound
	}
	return dir, nil
}

// List returns backups newest first with their last verification result.
func (m *Manager) List() ([]Status, error) {
	infos, err := List(m.root)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, len(infos))
	for i, in := range infos {
		out[i] = Status{Info: in}
		if r, ok := m.results[in.ID]; ok {
			r := r
			out[i].Verified = &r
		}
	}
	return out, nil
}

// Schedule returns the current interval (0 = off) and how many scheduled
// backups are kept.
func (m *Manager) Schedule() (time.Duration, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.every, m.keep
}

// Configure sets the schedule: a backup every interval (0 disables) and at most
// keep scheduled backups retained (manual ones are never pruned).
func (m *Manager) Configure(interval time.Duration, keep int) {
	m.mu.Lock()
	m.every = interval
	if keep > 0 {
		m.keep = keep
	}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Start runs the schedule loop until ctx ends.
func (m *Manager) Start(ctx context.Context) {
	go func() {
		for {
			every, _ := m.Schedule()
			var tick <-chan time.Time
			var t *time.Timer
			if every > 0 {
				t = time.NewTimer(every)
				tick = t.C
			}
			select {
			case <-ctx.Done():
				if t != nil {
					t.Stop()
				}
				return
			case <-m.wake: // schedule changed: restart the timer
				if t != nil {
					t.Stop()
				}
			case <-tick:
				if _, err := m.Create(KindScheduled, "scheduler"); err != nil {
					log.Printf("scheduled backup failed: %v", err)
				} else {
					m.prune()
				}
			}
		}
	}()
}

// prune removes the oldest scheduled backups beyond the retention count.
func (m *Manager) prune() {
	_, keep := m.Schedule()
	infos, err := List(m.root)
	if err != nil {
		return
	}
	kept := 0
	for _, in := range infos { // newest first
		if in.Kind != KindScheduled {
			continue
		}
		if kept++; kept > keep {
			if err := os.RemoveAll(filepath.Join(m.root, in.ID)); err != nil {
				log.Printf("backup retention: could not remove %s: %v", in.ID, err)
				continue
			}
			m.mu.Lock()
			delete(m.results, in.ID)
			m.mu.Unlock()
		}
	}
}
