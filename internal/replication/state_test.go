package replication

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStateStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	st, err := LoadStateStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Get(); got != (State{}) {
		t.Fatalf("fresh node should have zero state, got %+v", got)
	}
	want := State{Epoch: 3, Role: RoleLeader, ServedEpoch: 3, PromotedAtLSN: 42}
	if err := st.Set(want); err != nil {
		t.Fatal(err)
	}
	re, err := LoadStateStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := re.Get(); got != want {
		t.Fatalf("reopened state = %+v, want %+v", got, want)
	}
}

func TestStateStoreNeverMovesBackwards(t *testing.T) {
	st, _ := LoadStateStore(t.TempDir())
	if err := st.Set(State{Epoch: 5, Role: RoleFollower}); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(State{Epoch: 4, Role: RoleFollower}); !errors.Is(err, ErrEpochRegression) {
		t.Fatalf("lower epoch: err = %v, want ErrEpochRegression", err)
	}
	// At one epoch a follower cannot become leader or fenced by update; that
	// would be a promotion without an epoch bump.
	if err := st.Set(State{Epoch: 5, Role: RoleLeader}); !errors.Is(err, ErrEpochRegression) {
		t.Fatalf("same-epoch follower->leader: err = %v, want ErrEpochRegression", err)
	}
	if got := st.Epoch(); got != 5 {
		t.Fatalf("epoch changed by refused updates: %d", got)
	}
	if err := st.Set(State{Epoch: 6, Role: RoleLeader}); err != nil {
		t.Fatalf("higher epoch should be allowed: %v", err)
	}
	if err := st.Set(State{Epoch: 6, Role: RoleFenced}); err != nil {
		t.Fatalf("leader -> fenced at same epoch should be allowed: %v", err)
	}
	if err := st.Set(State{Epoch: 6, Role: RoleLeader}); !errors.Is(err, ErrEpochRegression) {
		t.Fatalf("fenced -> leader at same epoch: err = %v, want ErrEpochRegression", err)
	}
}

// A crash during Set leaves a temp file behind (written, not yet renamed).
// The previous state must still load, and a later Set must still work.
func TestStateStoreSurvivesCrashMidWrite(t *testing.T) {
	dir := t.TempDir()
	st, _ := LoadStateStore(dir)
	if err := st.Set(State{Epoch: 2, Role: RoleFollower}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StateFile+".tmp"), []byte(`{"epoch":9,"ro`), 0600); err != nil {
		t.Fatal(err)
	}
	re, err := LoadStateStore(dir)
	if err != nil {
		t.Fatalf("stray temp file must not break loading: %v", err)
	}
	if re.Epoch() != 2 {
		t.Fatalf("epoch = %d, want 2 (the temp file is not committed)", re.Epoch())
	}
	if err := re.Set(State{Epoch: 3, Role: RoleFollower}); err != nil {
		t.Fatalf("Set after stray temp file: %v", err)
	}
}

func TestStateStoreRefusesCorruptFile(t *testing.T) {
	for name, body := range map[string]string{
		"garbage":      "not json",
		"unknown role": `{"epoch":2,"role":"king"}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, StateFile), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadStateStore(dir); err == nil {
			t.Fatalf("%s: a node must not guess its epoch from a corrupt state file", name)
		}
	}
}

func TestResolveRole(t *testing.T) {
	cases := []struct {
		name    string
		st      State
		flag    Role
		want    Role
		wantErr bool
	}{
		{"fresh leader", State{}, RoleLeader, RoleLeader, false},
		{"fresh follower", State{}, RoleFollower, RoleFollower, false},
		{"promoted node restarted with follower flag stays leader", State{Epoch: 2, Role: RoleLeader}, RoleFollower, RoleLeader, false},
		{"fenced node restarted as leader stays fenced", State{Epoch: 2, Role: RoleFenced}, RoleLeader, RoleFenced, false},
		{"fenced node cannot restart as follower", State{Epoch: 2, Role: RoleFenced}, RoleFollower, "", true},
		{"follower cannot be started as leader by flag", State{Epoch: 2, Role: RoleFollower}, RoleLeader, "", true},
		{"follower restarts as follower", State{Epoch: 2, Role: RoleFollower}, RoleFollower, RoleFollower, false},
	}
	for _, tc := range cases {
		got, err := ResolveRole(tc.st, tc.flag)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, err=%v)", tc.name, got, err, tc.want, tc.wantErr)
		}
	}
}
