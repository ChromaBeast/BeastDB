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

// Status is a listed backup plus its last known verification and off-host copy.
type Status struct {
	Info
	Verified *Result `json:"verified"` // nil until verified since this process started
	// Offsite is the last off-host copy attempt since this process started; nil
	// when none was made (no destination, paused, or an older backup).
	Offsite *Result `json:"offsite"`
}

// Manager owns the backup directory, serialises backups and runs the optional
// schedule.
type Manager struct {
	root string
	eng  Snapshotter

	run     sync.Mutex // held while a backup is being created
	copyMu  sync.Mutex // one off-host copy at a time
	mu      sync.Mutex // guards results and schedule fields
	results map[string]Result
	offsite map[string]Result
	dest    Destination // nil when no off-host destination is configured
	copyOn  bool
	onCopy  func(id string, r Result) // observes automatic copies (audit log)
	every   time.Duration
	keep    int
	wake    chan struct{}
}

// NewManager returns a Manager storing backups under root.
func NewManager(eng Snapshotter, root string) *Manager {
	return &Manager{root: root, eng: eng, results: map[string]Result{}, offsite: map[string]Result{}, keep: 7, copyOn: true, wake: make(chan struct{}, 1)}
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
	if m.record(info.ID).OK {
		m.copyOut(info.ID) // never ship a bundle that failed its own check
	}
	return info, nil
}

// SetDestination sets where finished backups are also copied. Call before
// Start; nil means local only.
func (m *Manager) SetDestination(d Destination) {
	m.mu.Lock()
	m.dest = d
	m.mu.Unlock()
}

// SetOffsite turns the off-host copy of new backups on or off at run time.
func (m *Manager) SetOffsite(on bool) {
	m.mu.Lock()
	m.copyOn = on
	m.mu.Unlock()
}

// DestinationInfo describes the configured destination without any secret.
type DestinationInfo struct {
	Kind    string `json:"kind"`
	Where   string `json:"where"`
	Enabled bool   `json:"enabled"`
}

// DestinationInfo returns the destination, or nil when none is configured.
func (m *Manager) DestinationInfo() *DestinationInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dest == nil {
		return nil
	}
	return &DestinationInfo{Kind: m.dest.Kind(), Where: m.dest.Where(), Enabled: m.copyOn}
}

// ErrNoDestination is returned by CopyOffsite when none is configured.
var ErrNoDestination = errors.New("no off-host destination is configured")

// copyOut copies a backup to the destination when one is configured and enabled.
func (m *Manager) copyOut(id string) {
	m.mu.Lock()
	d, on := m.dest, m.copyOn
	m.mu.Unlock()
	if d == nil || !on {
		return
	}
	r, err := m.CopyOffsite(id)
	m.mu.Lock()
	fn := m.onCopy
	m.mu.Unlock()
	if fn != nil && err == nil {
		fn(id, r)
	}
}

// SetCopyObserver registers fn to be told the outcome of each automatic
// off-host copy (not the ones an admin asks for, whose handler reports them).
func (m *Manager) SetCopyObserver(fn func(id string, r Result)) {
	m.mu.Lock()
	m.onCopy = fn
	m.mu.Unlock()
}

// CopyOffsite copies one backup to the destination now (also used to retry a
// failed copy) and records the outcome. It does not require the automatic
// copy to be switched on, since an admin asked for it.
func (m *Manager) CopyOffsite(id string) (Result, error) {
	m.mu.Lock()
	d := m.dest
	m.mu.Unlock()
	if d == nil {
		return Result{}, ErrNoDestination
	}
	dir, err := m.Path(id)
	if err != nil {
		return Result{}, err
	}
	m.copyMu.Lock()
	defer m.copyMu.Unlock()
	r := Result{OK: true}
	if err := d.Put(context.Background(), id, dir); err != nil {
		r.OK, r.Error = false, err.Error()
		log.Printf("backup %s: off-host copy failed: %v", id, err)
	}
	r.At = time.Now().UTC()
	m.mu.Lock()
	m.offsite[id] = r
	m.mu.Unlock()
	return r, nil
}

func (m *Manager) record(id string) Result {
	r := Result{OK: true, At: time.Now().UTC()}
	if _, err := m.verify(filepath.Join(m.root, id)); err != nil {
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
		if r, ok := m.offsite[in.ID]; ok {
			r := r
			out[i].Offsite = &r
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
			delete(m.offsite, in.ID)
			m.mu.Unlock()
		}
	}
}

// verify checks a bundle, authenticating the pages of an encrypted one with the
// engine's own key.
func (m *Manager) verify(dir string) (Manifest, error) {
	if enc, ok := m.eng.(Encrypted); ok {
		return VerifyWith(dir, enc.VerifyCipher())
	}
	return Verify(dir)
}
