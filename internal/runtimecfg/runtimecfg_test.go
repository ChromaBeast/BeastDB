package runtimecfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var defaults = Values{WALCheckpointMB: 64, LoginMaxAttempts: 5, LoginWindowSeconds: 30, BackupKeep: 7}

func TestUpdatePersistsAppliesAndOverridesNextStart(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir, defaults)
	if err != nil {
		t.Fatal(err)
	}
	var seen []Values
	s.Subscribe(func(v Values) { seen = append(seen, v) })
	if len(seen) != 1 || seen[0] != defaults {
		t.Fatalf("subscriber must start in sync: %+v", seen)
	}

	changes, err := s.Update(map[string]int{"wal_checkpoint_mb": 128})
	if err != nil || len(changes) != 1 || changes[0] != (Change{"wal_checkpoint_mb", 64, 128}) {
		t.Fatalf("%+v %v", changes, err)
	}
	if len(seen) != 2 || seen[1].WALCheckpointMB != 128 || seen[1].LoginMaxAttempts != 5 {
		t.Fatalf("subscriber not notified correctly: %+v", seen)
	}
	if info, _ := os.Stat(filepath.Join(dir, FileName)); info == nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings file must exist with 0600: %v", info)
	}

	// A restart with different flag defaults still sees the saved override.
	s2, err := Load(dir, Values{WALCheckpointMB: 32, LoginMaxAttempts: 9, LoginWindowSeconds: 60, BackupKeep: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Get(); got.WALCheckpointMB != 128 {
		t.Fatalf("override lost across restart: %+v", got)
	}
}

func TestInvalidChangesAreRejectedAndChangeNothing(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(dir, defaults)
	notified := 0
	s.Subscribe(func(Values) { notified++ })
	notified = 0

	for name, patch := range map[string]map[string]int{
		"below range":          {"login_max_attempts": 0},
		"above range":          {"wal_checkpoint_mb": 1 << 30},
		"unknown":              {"admin_password": 1},
		"mixed":                {"wal_checkpoint_mb": 100, "login_window_seconds": 1},
		"too frequent backups": {"backup_interval_minutes": 2},
	} {
		if _, err := s.Update(patch); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	if s.Get() != defaults || notified != 0 {
		t.Fatalf("rejected updates must not change or notify: %+v %d", s.Get(), notified)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Fatal("rejected updates must not write the file")
	}
	if ch, err := s.Update(map[string]int{"wal_checkpoint_mb": 64}); err != nil || ch != nil || notified != 0 {
		t.Fatal("setting the current value is a no-op")
	}
}

func TestBadFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte(`{"login_max_attempts": 9999}`), 0o600)
	s, err := Load(dir, defaults)
	if err == nil || !strings.Contains(err.Error(), "login_max_attempts") {
		t.Fatalf("invalid file should be reported: %v", err)
	}
	if s.Get() != defaults {
		t.Fatal("invalid file must not change the running values")
	}
}
