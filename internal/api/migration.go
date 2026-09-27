package api

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/ChromaBeast/beastdb/internal/index"
	"github.com/ChromaBeast/beastdb/internal/storage"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

// isAllZeros checks if the entire byte slice is zero-filled.
func isAllZeros(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

// isLegacyV0Page tests if page data conforms to a legacy v0 B+ tree node (Page 0).
func isLegacyV0Page(data []byte) bool {
	if len(data) < 20 {
		return false
	}
	nodeType := data[0]
	isRoot := data[1]
	keyCount := binary.LittleEndian.Uint16(data[2:4])
	return (nodeType == index.NodeTypeLeaf || nodeType == index.NodeTypeInternal) &&
		isRoot <= 1 &&
		keyCount <= 250
}

// findLegacyRootPageID locates the root node in a legacy v0 database.
func findLegacyRootPageID(bpm *storage.BufferPoolManager, numPages uint64) uint64 {
	p0, err := bpm.FetchPage(0)
	if err == nil {
		h := index.ReadNodeHeader(p0.Data())
		_ = bpm.UnpinPage(0, false)
		if h.IsRoot {
			return 0
		}
	}
	for i := uint64(1); i < numPages; i++ {
		p, err := bpm.FetchPage(i)
		if err != nil {
			continue
		}
		data := p.Data()
		nodeType := data[0]
		isRoot := data[1] == 1
		_ = bpm.UnpinPage(i, false)
		if (nodeType == index.NodeTypeLeaf || nodeType == index.NodeTypeInternal) && isRoot {
			return i
		}
	}
	return 0
}

// migrateV0ToV1 safely extracts all records from a legacy v0 database and installs a fresh v1 format.
func migrateV0ToV1(dbPath, walPath string, poolSize int) error {
	log.Printf("Detected legacy BeastDB storage format at %q. Initiating automatic migration to v1...", dbPath)

	disk, err := storage.OpenDiskManager(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open legacy database: %w", err)
	}
	bpm := storage.NewBufferPoolManager(disk, poolSize)

	rootID := findLegacyRootPageID(bpm, disk.NumPages())
	tree := index.OpenBPlusTree(rootID, bpm)

	cursor, err := tree.Scan(0, math.MaxUint64)
	if err != nil {
		_ = disk.Close()
		return fmt.Errorf("failed to scan legacy tree: %w", err)
	}

	records := make(map[uint64][]byte)
	for {
		key, rid, ok, err := cursor.Next()
		if !ok || err != nil {
			break
		}
		page, err := bpm.FetchPage(rid.PageID)
		if err != nil {
			continue
		}
		tuple, err := page.GetTuple(rid.SlotID)
		_ = bpm.UnpinPage(rid.PageID, false)
		if err != nil {
			continue
		}
		valCopy := make([]byte, len(tuple))
		copy(valCopy, tuple)
		records[key] = valCopy
	}
	_ = cursor.Close()
	_ = disk.Close()

	if _, err := os.Stat(walPath); err == nil {
		replayLegacyWAL(walPath, records)
	}

	tmpDB := dbPath + ".migrated.tmp"
	tmpWAL := walPath + ".migrated.tmp"
	_ = os.Remove(tmpDB)
	_ = os.Remove(tmpWAL)

	eng, err := NewEngine(tmpDB, tmpWAL, poolSize)
	if err != nil {
		return fmt.Errorf("failed to create temporary v1 engine: %w", err)
	}

	for k, v := range records {
		if err := eng.Put(k, v); err != nil {
			_ = eng.Close()
			_ = os.Remove(tmpDB)
			_ = os.Remove(tmpWAL)
			return fmt.Errorf("failed to insert migrated record %d: %w", k, err)
		}
	}
	if err := eng.Close(); err != nil {
		return fmt.Errorf("failed to close temporary v1 engine: %w", err)
	}

	bakDB := dbPath + ".v0.bak"
	bakWAL := walPath + ".v0.bak"
	_ = os.Remove(bakDB)
	_ = os.Remove(bakWAL)

	if err := os.Rename(dbPath, bakDB); err != nil {
		return fmt.Errorf("failed to backup legacy database: %w", err)
	}
	if _, err := os.Stat(walPath); err == nil {
		_ = os.Rename(walPath, bakWAL)
	}
	if err := os.Rename(tmpDB, dbPath); err != nil {
		return fmt.Errorf("failed to install migrated database: %w", err)
	}
	_ = os.Rename(tmpWAL, walPath)

	log.Printf("Successfully migrated %d records to v1 storage format. Legacy backup saved to %q.", len(records), bakDB)
	return nil
}

// replayLegacyWAL reads mutations from a legacy WAL and applies them to the in-memory map.
func replayLegacyWAL(walPath string, records map[uint64][]byte) {
	_, _ = wal.Replay(walPath, func(rec *wal.Record) error {
		switch rec.Type {
		case wal.OpPut:
			if len(rec.Key) == 8 {
				k := binary.LittleEndian.Uint64(rec.Key)
				records[k] = rec.Value
			}
		case wal.OpDelete:
			if len(rec.Key) == 8 {
				k := binary.LittleEndian.Uint64(rec.Key)
				delete(records, k)
			}
		case wal.OpBatch:
			ops, err := wal.DecodeBatchPayload(rec.Value)
			if err == nil {
				for _, op := range ops {
					if len(op.Key) == 8 {
						k := binary.LittleEndian.Uint64(op.Key)
						if op.Type == wal.OpPut {
							records[k] = op.Value
						} else if op.Type == wal.OpDelete {
							delete(records, k)
						}
					}
				}
			}
		}
		return nil
	})
}
