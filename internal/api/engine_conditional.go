package api

import (
	"bytes"
	"errors"

	"github.com/ChromaBeast/beastdb/internal/index"
)

// ErrConditionFailed indicates a write precondition did not hold; nothing was written.
var ErrConditionFailed = errors.New("write precondition failed")

// ConditionKind selects the check applied to a key before a conditional write.
type ConditionKind byte

const (
	// CondNone applies no check.
	CondNone ConditionKind = iota
	// CondAbsent requires the key to not exist.
	CondAbsent
	// CondPresent requires the key to exist.
	CondPresent
	// CondValueEquals requires the key to exist with exactly the given value.
	CondValueEquals
)

// Condition is a precondition on the current state of a key.
type Condition struct {
	Kind  ConditionKind
	Value []byte
}

// KeyCondition binds a Condition to a key, for use with BatchWriteIf.
type KeyCondition struct {
	Key  uint64
	Cond Condition
}

// checkConditionLocked evaluates cond against the current record. Caller holds e.mu.
func (e *Engine) checkConditionLocked(key uint64, cond Condition) error {
	if cond.Kind == CondNone {
		return nil
	}
	rid, err := e.tree.Find(key)
	if err == index.ErrKeyNotFound {
		if cond.Kind == CondAbsent {
			return nil
		}
		return ErrConditionFailed
	}
	if err != nil {
		return err
	}
	switch cond.Kind {
	case CondAbsent:
		return ErrConditionFailed
	case CondPresent:
		return nil
	case CondValueEquals:
		cur, err := e.ReadTuple(rid)
		if err != nil {
			return err
		}
		if !bytes.Equal(cur, cond.Value) {
			return ErrConditionFailed
		}
		return nil
	default:
		return errors.New("engine: unknown condition kind")
	}
}

// PutIf writes value only if cond holds for key, atomically with the check.
func (e *Engine) PutIf(key uint64, value []byte, cond Condition) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	if len(value) > MaxRecordSize {
		return ErrRecordTooLarge
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	if err := e.checkConditionLocked(key, cond); err != nil {
		return err
	}
	return e.putLocked(key, value)
}

// DeleteIf removes key only if cond holds, atomically with the check.
func (e *Engine) DeleteIf(key uint64, cond Condition) error {
	if e.IsReadOnly() {
		return ErrReadOnlyReplica
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	if err := e.checkConditionLocked(key, cond); err != nil {
		return err
	}
	return e.deleteLocked(key)
}
