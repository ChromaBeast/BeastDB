package index

import (
	"encoding/binary"
	"errors"
)

// ErrBadSnapshot reports a search index snapshot that does not decode.
var ErrBadSnapshot = errors.New("index: malformed search index snapshot")

// Snapshot serializes the index as: document count (8), then per document its
// primary key (8), token count (4) and tokens (8 each). Postings are rebuilt
// from the documents on load.
func (idx *SecondaryIndex) Snapshot() []byte {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	size := 8
	for _, toks := range idx.docTokens {
		size += 12 + 8*len(toks)
	}
	buf := make([]byte, 8, size)
	binary.LittleEndian.PutUint64(buf, uint64(len(idx.docTokens)))
	for key, toks := range idx.docTokens {
		buf = binary.LittleEndian.AppendUint64(buf, key)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(toks)))
		for _, t := range toks {
			buf = binary.LittleEndian.AppendUint64(buf, t)
		}
	}
	return buf
}

// LoadSnapshot replaces the index contents with a Snapshot. On error the
// index is left empty.
func (idx *SecondaryIndex) LoadSnapshot(data []byte) error {
	fresh := NewSecondaryIndex()
	if len(data) < 8 {
		return ErrBadSnapshot
	}
	n := binary.LittleEndian.Uint64(data)
	off := 8
	for i := uint64(0); i < n; i++ {
		if off+12 > len(data) {
			return ErrBadSnapshot
		}
		key := binary.LittleEndian.Uint64(data[off:])
		cnt := int(binary.LittleEndian.Uint32(data[off+8:]))
		off += 12
		if cnt < 0 || cnt > (len(data)-off)/8 {
			return ErrBadSnapshot
		}
		toks := make([]uint64, cnt)
		for j := range toks {
			toks[j] = binary.LittleEndian.Uint64(data[off:])
			off += 8
		}
		fresh.Index(key, toks)
	}
	if off != len(data) {
		return ErrBadSnapshot
	}
	idx.mu.Lock()
	idx.postings, idx.docTokens = fresh.postings, fresh.docTokens
	idx.mu.Unlock()
	return nil
}
