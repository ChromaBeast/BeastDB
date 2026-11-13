package replication

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Role is a node's persisted place in the replication topology.
type Role string

const (
	// RoleLeader accepts client writes and serves the WAL to followers.
	RoleLeader Role = "leader"
	// RoleFollower is read-only and applies a leader's WAL.
	RoleFollower Role = "follower"
	// RoleFenced is a former leader that saw a higher epoch. It is read-only
	// and stays so across restarts until an operator rebuilds it.
	RoleFenced Role = "fenced"
)

// StateFile is the name of the persisted state inside the data directory.
const StateFile = "cluster-state.json"

// State is the durable replication state of one node. The epoch only ever
// increases; a role change at the same epoch is limited to leader -> fenced.
type State struct {
	// Epoch is the highest leader term this node has seen or led.
	Epoch uint64 `json:"epoch"`
	Role  Role   `json:"role"`
	// ServedEpoch is the epoch this node led at. A fenced leader's Epoch moves
	// up to the fencing epoch, but what it already wrote was written at
	// ServedEpoch, so that is what it stamps on records it still serves.
	ServedEpoch uint64 `json:"servedEpoch,omitempty"`
	// FencedFor is the replica that fenced this node (RoleFenced only). A
	// different replica asking to fence the same epoch is refused, so two
	// followers cannot both be promoted at one epoch.
	FencedFor string `json:"fencedFor,omitempty"`
	// PromotedAtLSN is the WAL position at which this node became leader by
	// promotion (0 if it never was promoted).
	PromotedAtLSN uint64 `json:"promotedAtLSN,omitempty"`
}

// ErrEpochRegression is returned when an update would lower the epoch or make
// a disallowed role change.
var ErrEpochRegression = errors.New("replication: epoch cannot go backwards")

// StateStore persists State with write-to-temp, fsync, rename, so a crash
// leaves either the old or the new state, never a torn one.
type StateStore struct {
	mu    sync.Mutex
	path  string
	state State
}

// LoadStateStore reads <dir>/cluster-state.json. A missing file is a fresh
// node (epoch 0, no role). A present but unreadable file is an error: the
// node must not guess its epoch.
func LoadStateStore(dir string) (*StateStore, error) {
	st := &StateStore{path: filepath.Join(dir, StateFile)}
	data, err := os.ReadFile(st.path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &st.state); err != nil {
		return nil, fmt.Errorf("replication: %s is corrupt: %w", st.path, err)
	}
	switch st.state.Role {
	case RoleLeader, RoleFollower, RoleFenced:
	default:
		return nil, fmt.Errorf("replication: %s has unknown role %q", st.path, st.state.Role)
	}
	return st, nil
}

// Get returns a copy of the current state.
func (s *StateStore) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Epoch returns the current epoch.
func (s *StateStore) Epoch() uint64 { return s.Get().Epoch }

// Set durably replaces the state if it is a legal successor of the current
// one: the epoch may not decrease, and at an equal epoch the role may only
// stay the same or move from leader to fenced.
func (s *StateStore) Set(next State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.state
	if next.Epoch < cur.Epoch {
		return fmt.Errorf("%w: have %d, asked for %d", ErrEpochRegression, cur.Epoch, next.Epoch)
	}
	if next.Epoch == cur.Epoch && cur.Role != "" && next.Role != cur.Role &&
		!(cur.Role == RoleLeader && next.Role == RoleFenced) {
		return fmt.Errorf("%w: %s -> %s at epoch %d", ErrEpochRegression, cur.Role, next.Role, cur.Epoch)
	}
	if err := writeAtomic(s.path, next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func writeAtomic(path string, st State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Persist the rename itself; not supported on every platform, so best effort.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// ResolveRole decides the role a node starts in from its persisted state and
// the -role flag. The persisted state wins where ignoring it would break
// fencing:
//
//   - fenced stays fenced (a fenced node refuses to start as a follower, since
//     its data may hold writes the new leader never saw);
//   - a promoted leader stays leader when restarted with -role follower;
//   - a persisted follower cannot be started as leader by flag: it must be
//     promoted, which bumps the epoch.
func ResolveRole(st State, flag Role) (Role, error) {
	switch st.Role {
	case RoleFenced:
		if flag == RoleFollower {
			return "", fmt.Errorf("node was fenced at epoch %d: its data may hold writes the current leader never saw; wipe the data directory to rejoin it as a follower", st.Epoch)
		}
		return RoleFenced, nil
	case RoleLeader:
		return RoleLeader, nil
	case RoleFollower:
		if flag == RoleLeader {
			return "", fmt.Errorf("node is a follower at epoch %d: promote it (POST /api/replication/promote) instead of starting it with -role leader", st.Epoch)
		}
		return RoleFollower, nil
	}
	return flag, nil
}
