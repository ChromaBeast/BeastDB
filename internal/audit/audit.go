// Package audit is an append-only, hash-chained log of administrative actions
// (sign-ins, key changes, deletes, checkpoints, settings changes).
//
// Each record carries the SHA-256 of the previous record, so editing, removing
// or reordering an earlier line breaks every hash after it and Verify reports
// where. The chain detects accidental damage and casual tampering; it is not a
// defence against someone who can rewrite the whole file, and nothing here is
// replicated off the host. Secrets and record values are never written.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileName is the audit log's name inside the data directory.
const FileName = "audit.log"

// Entry is one audit record. Prev and Hash form the chain.
type Entry struct {
	Seq    uint64    `json:"seq"`
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target,omitempty"`
	Detail string    `json:"detail,omitempty"`
	IP     string    `json:"ip,omitempty"`
	Prev   string    `json:"prev"`
	Hash   string    `json:"hash"`
}

// Actions recorded by the server.
const (
	ActionLoginOK        = "login.ok"
	ActionLoginFailed    = "login.failed"
	ActionLogout         = "logout"
	ActionTokenCreate    = "token.create"
	ActionTokenRevoke    = "token.revoke"
	ActionUserDelete     = "user.delete"
	ActionRecordPut      = "record.put"
	ActionRecordDelete   = "record.delete"
	ActionCheckpoint     = "checkpoint"
	ActionSettingChange  = "settings.update"
	ActionBackupCreate   = "backup.create"
	ActionBackupVerify   = "backup.verify"
	ActionBackupDownload = "backup.download"
	ActionPromote        = "replication.promote"
)

const maxField = 512

// Log is the audit log. A nil *Log is valid and discards everything, so callers
// never need to check whether auditing is configured.
type Log struct {
	mu   sync.Mutex
	f    *os.File
	path string
	seq  uint64
	last string // hash of the last record
}

// Open opens (creating if needed) dir/audit.log, verifies the existing chain
// and positions for appending. It fails if the existing chain is broken, so a
// tampered log is noticed at startup instead of being silently extended.
func Open(dir string) (*Log, error) {
	path := filepath.Join(dir, FileName)
	l := &Log{path: path}
	if _, err := os.Stat(path); err == nil {
		n, last, verr := verifyFile(path)
		if verr != nil {
			return nil, fmt.Errorf("audit log %s is damaged: %w", path, verr)
		}
		l.seq, l.last = n, last
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

// Close releases the file.
func (l *Log) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.f.Close()
	l.f = nil
	return err
}

func clip(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > maxField {
		return s[:maxField] + "…"
	}
	return s
}

func hashOf(e Entry) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(append([]byte(e.Prev+"|"), b...))
	return hex.EncodeToString(sum[:])
}

// Record appends one entry and fsyncs it. It is best effort for callers: a
// write failure is logged and returned, but admin actions are not blocked on it.
func (l *Log) Record(actor, action, target, detail, ip string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return errors.New("audit log closed")
	}
	e := Entry{
		Seq: l.seq + 1, Time: time.Now().UTC(),
		Actor: clip(actor), Action: action, Target: clip(target), Detail: clip(detail), IP: clip(ip),
		Prev: l.last,
	}
	e.Hash = hashOf(e)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := l.f.Write(append(b, '\n')); err != nil {
		log.Printf("audit: write failed: %v", err)
		return err
	}
	if err := l.f.Sync(); err != nil {
		log.Printf("audit: sync failed: %v", err)
		return err
	}
	l.seq, l.last = e.Seq, e.Hash
	return nil
}

// Query selects entries. Zero values mean no constraint.
type Query struct {
	// Before returns entries with Seq < Before (for paging back). 0 = newest.
	Before uint64
	// Limit caps the result; <=0 means 100, max 1000.
	Limit  int
	Action string
	Actor  string
}

// List returns matching entries newest first.
func (l *Log) List(q Query) ([]Entry, error) {
	if l == nil {
		return []Entry{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	all, err := readAll(l.path)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		e := all[i]
		if q.Before != 0 && e.Seq >= q.Before {
			continue
		}
		if q.Action != "" && e.Action != q.Action {
			continue
		}
		if q.Actor != "" && e.Actor != q.Actor {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Status reports the chain state for display.
type Status struct {
	Entries  uint64 `json:"entries"`
	Verified bool   `json:"verified"`
	// Error describes the first break in the chain when Verified is false.
	Error string `json:"error,omitempty"`
}

// Verify re-reads the whole file and checks every link.
func (l *Log) Verify() Status {
	if l == nil {
		return Status{Verified: true}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	n, _, err := verifyFile(l.path)
	if err != nil {
		return Status{Entries: n, Error: err.Error()}
	}
	return Status{Entries: n, Verified: true}
}

func readAll(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return out, fmt.Errorf("line %d: %w", len(out)+1, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// verifyFile checks sequence numbers, back-links and hashes. It returns the
// count of valid records and the last hash.
func verifyFile(path string) (uint64, string, error) {
	entries, err := readAll(path)
	if err != nil {
		return uint64(len(entries)), "", err
	}
	prev := ""
	for i, e := range entries {
		if e.Seq != uint64(i+1) {
			return uint64(i), prev, fmt.Errorf("record %d: sequence is %d, expected %d (a line was removed or reordered)", i+1, e.Seq, i+1)
		}
		if e.Prev != prev {
			return uint64(i), prev, fmt.Errorf("record %d: does not link to the previous record", e.Seq)
		}
		if hashOf(e) != e.Hash {
			return uint64(i), prev, fmt.Errorf("record %d: contents do not match their hash (edited)", e.Seq)
		}
		prev = e.Hash
	}
	return uint64(len(entries)), prev, nil
}
