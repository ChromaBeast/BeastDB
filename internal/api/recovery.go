package api

import (
	"encoding/binary"

	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// recoverFromWAL scans the WAL and reapplies any committed records with LSN > lastCheckpointLSN.
func (e *Engine) recoverFromWAL(lastCheckpointLSN uint64) error {
	replayedCount := 0
	var highestReplayedLSN uint64

	_, err := wal.Replay(e.wal.Path(), func(rec *wal.Record) error {
		if rec.LSN <= lastCheckpointLSN {
			return nil
		}
		if err := e.applyRecordLocked(rec); err != nil {
			return err
		}
		replayedCount++
		if rec.LSN > highestReplayedLSN {
			highestReplayedLSN = rec.LSN
		}
		return nil
	})
	if err != nil {
		return err
	}

	if replayedCount > 0 {
		if err := e.updateMeta(func(m *storage.MetaData) {
			m.LastCheckpointLSN = highestReplayedLSN
			m.ActiveDataPageID = e.activeDataPage
			m.RootPageID = e.tree.RootPageID()
		}); err != nil {
			return err
		}
		if err := e.bpm.FlushAll(); err != nil {
			return err
		}
	}

	return nil
}

// applyRecordLocked applies a single record during recovery without logging back to the WAL.
func (e *Engine) applyRecordLocked(rec *wal.Record) error {
	if rec.Type == wal.OpBatch {
		ops, err := wal.DecodeBatchPayload(rec.Value)
		if err != nil {
			return err
		}
		for _, op := range ops {
			subRec := &wal.Record{
				LSN:   rec.LSN,
				Type:  op.Type,
				Key:   op.Key,
				Value: op.Value,
			}
			if err := e.applyRecordLocked(subRec); err != nil {
				return err
			}
		}
		return nil
	}

	if len(rec.Key) < 8 {
		return nil
	}
	key := binary.LittleEndian.Uint64(rec.Key)

	switch rec.Type {
	case wal.OpPut:
		if err := e.applyPutInMemory(key, rec.Value); err != nil {
			return err
		}
		e.indexValueLocked(key, rec.Value)
		return nil

	case wal.OpDelete:
		if err := e.applyDeleteInMemory(key); err != nil {
			return err
		}
		e.unindexValueLocked(key)
		return nil

	default:
		return nil
	}
}
