package api

import (
	"encoding/binary"
	"errors"
	"hash/crc32"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Values up to MaxRecordSize live inline in one slotted-page tuple. Larger
// values, up to MaxValueSize, are stored as a chain of chunk tuples headed by a
// small manifest tuple; the B+ tree points at the manifest with largeSlotFlag
// set in the RID's slot ID. A 4 KB page holds at most ~1,000 slots, so bit 15
// is never used by an inline RID, and older binaries reading a flagged RID fail
// with ErrSlotOutOfBounds instead of returning the manifest as data.
const (
	// MaxValueSize is the largest value accepted by Put, PutIf and BatchWrite.
	MaxValueSize = 2 << 20
	// MaxBatchBytes bounds one BatchWrite's encoded WAL frame: its values plus 15 bytes per operation.
	MaxBatchBytes = 8 << 20

	largeSlotFlag = 0x8000
	chunkMagic    = 0xBC
	manifestMagic = 0xBD
	manifestV1    = 1
	chunkHeader   = 11 // magic(1) + next page(8) + next slot(2)
	manifestSize  = 24 // magic, version, length(4), crc32(4), chunks(4), first page(8), first slot(2)
	chunkPayload  = MaxRecordSize - chunkHeader
	maxChunks     = (MaxValueSize + chunkPayload - 1) / chunkPayload
)

// ErrCorruptValue reports a large-value chain that fails structural or CRC checks.
var ErrCorruptValue = errors.New("engine: stored value chain is corrupt")

func isLargeRID(rid storage.RID) bool { return rid.SlotID&largeSlotFlag != 0 }

func manifestOf(rid storage.RID) storage.RID {
	rid.SlotID &^= largeSlotFlag
	return rid
}

// insertValue stores value inline when it fits in one tuple, else as a chunk chain.
// Caller must hold e.mu exclusively.
func (e *Engine) insertValue(value []byte) (storage.RID, error) {
	if len(value) <= MaxRecordSize {
		return e.insertTuple(value)
	}
	n := (len(value) + chunkPayload - 1) / chunkPayload
	written := make([]storage.RID, 0, n+1)
	undo := func() {
		for _, r := range written {
			e.freeTuple(r)
		}
	}
	buf := make([]byte, MaxRecordSize)
	var next storage.RID // zero page ID ends the chain; page 0 is the meta page
	// Write the tail first so every chunk can name its successor.
	for i := n - 1; i >= 0; i-- {
		start := i * chunkPayload
		end := min(start+chunkPayload, len(value))
		t := buf[:chunkHeader+end-start]
		t[0] = chunkMagic
		binary.LittleEndian.PutUint64(t[1:9], next.PageID)
		binary.LittleEndian.PutUint16(t[9:11], next.SlotID)
		copy(t[chunkHeader:], value[start:end])
		rid, err := e.insertTuple(t)
		if err != nil {
			undo()
			return storage.RID{}, err
		}
		written = append(written, rid)
		next = rid
	}
	m := buf[:manifestSize]
	m[0], m[1] = manifestMagic, manifestV1
	binary.LittleEndian.PutUint32(m[2:6], uint32(len(value)))
	binary.LittleEndian.PutUint32(m[6:10], crc32.ChecksumIEEE(value))
	binary.LittleEndian.PutUint32(m[10:14], uint32(n))
	binary.LittleEndian.PutUint64(m[14:22], next.PageID)
	binary.LittleEndian.PutUint16(m[22:24], next.SlotID)
	rid, err := e.insertTuple(m)
	if err != nil {
		undo()
		return storage.RID{}, err
	}
	rid.SlotID |= largeSlotFlag
	return rid, nil
}

type manifest struct {
	length, crc, chunks uint32
	first               storage.RID
}

func (e *Engine) readManifest(rid storage.RID) (manifest, error) {
	m, err := e.readTuple(manifestOf(rid))
	if err != nil {
		return manifest{}, err
	}
	if len(m) != manifestSize || m[0] != manifestMagic || m[1] != manifestV1 {
		return manifest{}, ErrCorruptValue
	}
	out := manifest{
		length: binary.LittleEndian.Uint32(m[2:6]),
		crc:    binary.LittleEndian.Uint32(m[6:10]),
		chunks: binary.LittleEndian.Uint32(m[10:14]),
		first:  storage.RID{PageID: binary.LittleEndian.Uint64(m[14:22]), SlotID: binary.LittleEndian.Uint16(m[22:24])},
	}
	if out.length > MaxValueSize || out.chunks == 0 || out.chunks > maxChunks {
		return manifest{}, ErrCorruptValue
	}
	return out, nil
}

// walkChain visits each chunk of a large value in order and verifies the chain
// ends exactly where the manifest says. visit receives the chunk's payload.
func (e *Engine) walkChain(m manifest, visit func(rid storage.RID, payload []byte)) error {
	next := m.first
	for i := uint32(0); i < m.chunks; i++ {
		if next.PageID == 0 || isLargeRID(next) {
			return ErrCorruptValue
		}
		t, err := e.readTuple(next)
		if err != nil {
			return err
		}
		if len(t) < chunkHeader || t[0] != chunkMagic {
			return ErrCorruptValue
		}
		visit(next, t[chunkHeader:])
		next = storage.RID{PageID: binary.LittleEndian.Uint64(t[1:9]), SlotID: binary.LittleEndian.Uint16(t[9:11])}
	}
	if next.PageID != 0 {
		return ErrCorruptValue
	}
	return nil
}

// readValue returns a copy of the value at rid, reassembling and checking large values.
func (e *Engine) readValue(rid storage.RID) ([]byte, error) {
	if !isLargeRID(rid) {
		return e.readTuple(rid)
	}
	m, err := e.readManifest(rid)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, m.length)
	if err := e.walkChain(m, func(_ storage.RID, p []byte) { out = append(out, p...) }); err != nil {
		return nil, err
	}
	if uint32(len(out)) != m.length || crc32.ChecksumIEEE(out) != m.crc {
		return nil, ErrCorruptValue
	}
	return out, nil
}

// freeValue releases the tuples behind rid. A chain that fails any check,
// including its length and CRC, is left in place: a mis-linked chain can end
// cleanly after the right number of chunks while running through another
// key's tuples, and freeing it would destroy that key's value.
func (e *Engine) freeValue(rid storage.RID) {
	if !isLargeRID(rid) {
		e.freeTuple(rid)
		return
	}
	m, err := e.readManifest(rid)
	if err != nil {
		return
	}
	var chunks []storage.RID
	size, crc := 0, uint32(0)
	visit := func(r storage.RID, p []byte) {
		chunks = append(chunks, r)
		size += len(p)
		crc = crc32.Update(crc, crc32.IEEETable, p)
	}
	if e.walkChain(m, visit) != nil || uint32(size) != m.length || crc != m.crc {
		return
	}
	for _, r := range chunks {
		e.freeTuple(r)
	}
	e.freeTuple(manifestOf(rid))
}
