package api

import (
	"fmt"

	"github.com/ChromaBeast/beastdb/internal/wal"
)

// AppendEpochMarker logs the start of leader epoch in this node's WAL and
// returns the LSN it took. It is allowed while the engine is read-only: a node
// writes its marker as it becomes leader, before it opens for client writes.
// The marker is streamed to followers like any record.
func (e *Engine) AppendEpochMarker(epoch uint64) (uint64, error) {
	var lsn uint64
	err := e.commitReplicated(func() error {
		var err error
		lsn, err = e.epochLocked(wal.EncodeEpoch(epoch))
		return err
	})
	return lsn, err
}

// epochLocked logs an epoch marker exactly as given. It changes no data.
func (e *Engine) epochLocked(value []byte) (uint64, error) {
	if e.failed != nil {
		return 0, e.failed
	}
	if _, ok := wal.DecodeEpoch(value); !ok {
		return 0, wal.ErrBadEpochMarker
	}
	lsn, err := e.wal.Append(wal.OpEpoch, nil, value)
	if err != nil {
		return 0, e.halt(err)
	}
	e.queueNotify(lsn, wal.OpEpoch, nil, value)
	return lsn, nil
}

// EpochMarks lists the epoch markers still in this node's WAL (the active log
// and the one archived generation).
func (e *Engine) EpochMarks() ([]wal.EpochMark, error) {
	p := e.wal.Path()
	marks, err := wal.EpochMarks(e.wal.Cipher(), p+".bak", p)
	if err != nil {
		return nil, fmt.Errorf("read epoch markers: %w", err)
	}
	return marks, nil
}
