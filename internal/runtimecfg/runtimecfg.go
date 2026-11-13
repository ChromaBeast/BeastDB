// Package runtimecfg holds the few settings an admin may change while the
// server runs. Everything else stays a startup flag. Changes are validated,
// written atomically to runtime-settings.json in the data directory and applied
// to subscribers at once. On the next start the file overrides the matching
// flags, so a change made in Studio is not undone by a restart.
package runtimecfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileName is the persisted overrides file inside the data directory.
const FileName = "runtime-settings.json"

// Values are the adjustable settings.
type Values struct {
	// WALCheckpointMB is the WAL size in MiB that triggers an automatic checkpoint.
	WALCheckpointMB int `json:"wal_checkpoint_mb"`
	// LoginMaxAttempts failed sign-ins per client IP within LoginWindowSeconds are allowed.
	LoginMaxAttempts   int `json:"login_max_attempts"`
	LoginWindowSeconds int `json:"login_window_seconds"`
	// BackupIntervalMinutes between scheduled backups; 0 turns the schedule off.
	BackupIntervalMinutes int `json:"backup_interval_minutes"`
	// BackupKeep is how many scheduled backups are retained (manual ones are never pruned).
	BackupKeep int `json:"backup_keep"`
}

// Range is the inclusive bound of one setting.
type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Ranges lists the accepted bounds, keyed by JSON field name.
var Ranges = map[string]Range{
	"wal_checkpoint_mb":    {1, 65536},
	"login_max_attempts":   {1, 50},
	"login_window_seconds": {5, 3600},
	// 0 = off; 1-4 are rejected in Validate so a typo cannot flood the disk.
	"backup_interval_minutes": {0, 10080},
	"backup_keep":             {1, 365},
}

func fields(v *Values) map[string]*int {
	return map[string]*int{
		"wal_checkpoint_mb":       &v.WALCheckpointMB,
		"login_max_attempts":      &v.LoginMaxAttempts,
		"login_window_seconds":    &v.LoginWindowSeconds,
		"backup_interval_minutes": &v.BackupIntervalMinutes,
		"backup_keep":             &v.BackupKeep,
	}
}

// Validate checks every value against Ranges.
func Validate(v Values) error {
	for name, p := range fields(&v) {
		r := Ranges[name]
		if *p < r.Min || *p > r.Max {
			return fmt.Errorf("%s must be between %d and %d", name, r.Min, r.Max)
		}
	}
	if v.BackupIntervalMinutes > 0 && v.BackupIntervalMinutes < 5 {
		return fmt.Errorf("backup_interval_minutes must be 0 (off) or at least 5")
	}
	return nil
}

// Store is the live settings holder. Safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	path string
	v    Values
	subs []func(Values)
}

// Load returns a Store seeded with defaults (the startup flags) and then
// overridden by runtime-settings.json when it exists. An unreadable or invalid
// file is reported and ignored rather than preventing startup.
func Load(dir string, defaults Values) (*Store, error) {
	s := &Store{path: filepath.Join(dir, FileName), v: defaults}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	merged := defaults
	if err := json.Unmarshal(b, &merged); err != nil {
		return s, fmt.Errorf("%s: %w", FileName, err)
	}
	if err := Validate(merged); err != nil {
		return s, fmt.Errorf("%s: %w", FileName, err)
	}
	s.v = merged
	return s, nil
}

// Get returns the current values.
func (s *Store) Get() Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v
}

// Subscribe registers fn to be called with the new values after every update,
// and once immediately with the current values so it starts in sync.
func (s *Store) Subscribe(fn func(Values)) {
	s.mu.Lock()
	s.subs = append(s.subs, fn)
	v := s.v
	s.mu.Unlock()
	fn(v)
}

// Change describes one modified setting for auditing.
type Change struct {
	Name     string
	From, To int
}

// Update applies a partial change (JSON field name to new value). Unknown names
// and out-of-range values are rejected and nothing changes. On success it
// persists the file, notifies subscribers and returns what changed.
func (s *Store) Update(patch map[string]int) ([]Change, error) {
	s.mu.Lock()
	next := s.v
	nf := fields(&next)
	for name := range patch {
		if _, ok := nf[name]; !ok {
			s.mu.Unlock()
			return nil, fmt.Errorf("unknown setting %q", name)
		}
	}
	var changes []Change
	of := fields(&s.v)
	for name, val := range patch {
		if *of[name] != val {
			changes = append(changes, Change{Name: name, From: *of[name], To: val})
		}
		*nf[name] = val
	}
	if err := Validate(next); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if len(changes) == 0 {
		s.mu.Unlock()
		return nil, nil
	}
	if err := writeAtomic(s.path, next); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("could not save settings: %w", err)
	}
	s.v = next
	subs := append([]func(Values){}, s.subs...)
	s.mu.Unlock()
	for _, fn := range subs {
		fn(next)
	}
	return changes, nil
}

func writeAtomic(path string, v Values) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
