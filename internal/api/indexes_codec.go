package api

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// IndexDef declares one single-field secondary index (ADR 0005).
type IndexDef struct {
	Name      string
	Prefix    uint8  // collection prefix (ADR 0002)
	Field     string // dotted JSON object path
	Kind      int    // index.KindInt or index.KindString
	CreatedAt int64  // unix seconds
}

var indexCatalogMagic = [4]byte{'B', 'I', 'D', 'X'}

const indexCatalogVersion byte = 1

// ErrCorruptIndexCatalog means the index catalog record exists but cannot be decoded.
var ErrCorruptIndexCatalog = errors.New("indexes: catalog record is corrupt")

// encodeIndexCatalog serializes defs: magic, version, uvarint count, then per
// entry uvarint name length, name, prefix, kind, uvarint path length, path,
// int64 created-at (little endian).
func encodeIndexCatalog(defs []IndexDef) []byte {
	buf := make([]byte, 0, 16+len(defs)*48)
	buf = append(buf, indexCatalogMagic[:]...)
	buf = append(buf, indexCatalogVersion)
	buf = binary.AppendUvarint(buf, uint64(len(defs)))
	for _, d := range defs {
		buf = binary.AppendUvarint(buf, uint64(len(d.Name)))
		buf = append(buf, d.Name...)
		buf = append(buf, d.Prefix, byte(d.Kind))
		buf = binary.AppendUvarint(buf, uint64(len(d.Field)))
		buf = append(buf, d.Field...)
		buf = binary.LittleEndian.AppendUint64(buf, uint64(d.CreatedAt))
	}
	return buf
}

// decodeIndexCatalog strictly parses an index catalog record.
func decodeIndexCatalog(b []byte) ([]IndexDef, error) {
	bad := func(why string) ([]IndexDef, error) {
		return nil, fmt.Errorf("%w: %s", ErrCorruptIndexCatalog, why)
	}
	if len(b) < 6 || [4]byte(b[:4]) != indexCatalogMagic {
		return bad("bad magic")
	}
	if b[4] != indexCatalogVersion {
		return bad(fmt.Sprintf("unsupported version %d", b[4]))
	}
	b = b[5:]
	n, k := binary.Uvarint(b)
	if k <= 0 || n > maxIndexes {
		return bad("bad count")
	}
	b = b[k:]
	readStr := func(limit uint64) (string, bool) {
		l, k := binary.Uvarint(b)
		if k <= 0 || l > limit || uint64(len(b)-k) < l {
			return "", false
		}
		s := string(b[k : k+int(l)])
		b = b[k+int(l):]
		return s, true
	}
	out := make([]IndexDef, 0, n)
	for i := uint64(0); i < n; i++ {
		name, ok := readStr(maxCollectionName)
		if !ok || len(b) < 2 {
			return bad("truncated entry")
		}
		prefix, kind := b[0], int(b[1])
		b = b[2:]
		field, ok := readStr(maxFieldPath)
		if !ok || len(b) < 8 {
			return bad("truncated field")
		}
		created := int64(binary.LittleEndian.Uint64(b[:8]))
		b = b[8:]
		out = append(out, IndexDef{Name: name, Prefix: prefix, Field: field, Kind: kind, CreatedAt: created})
	}
	if len(b) != 0 {
		return bad("trailing bytes")
	}
	return out, nil
}
