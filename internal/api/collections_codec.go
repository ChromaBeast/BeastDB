package api

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Collection binds a name to one 8-bit key prefix (ADR 0002).
type Collection struct {
	Name      string
	Prefix    uint8
	CreatedAt int64 // unix seconds
}

var catalogMagic = [4]byte{'B', 'C', 'A', 'T'}

const catalogVersion byte = 1

// ErrCorruptCatalog means the reserved catalog record exists but cannot be decoded.
var ErrCorruptCatalog = errors.New("collections: catalog record is corrupt")

// encodeCatalog serializes entries: magic, version, uvarint count, then per
// entry prefix, uvarint name length, name, int64 created-at (little endian).
func encodeCatalog(entries []Collection) []byte {
	buf := make([]byte, 0, 16+len(entries)*24)
	buf = append(buf, catalogMagic[:]...)
	buf = append(buf, catalogVersion)
	buf = binary.AppendUvarint(buf, uint64(len(entries)))
	for _, c := range entries {
		buf = append(buf, c.Prefix)
		buf = binary.AppendUvarint(buf, uint64(len(c.Name)))
		buf = append(buf, c.Name...)
		buf = binary.LittleEndian.AppendUint64(buf, uint64(c.CreatedAt))
	}
	return buf
}

// decodeCatalog strictly parses a catalog record.
func decodeCatalog(b []byte) ([]Collection, error) {
	bad := func(why string) ([]Collection, error) {
		return nil, fmt.Errorf("%w: %s", ErrCorruptCatalog, why)
	}
	if len(b) < 6 || [4]byte(b[:4]) != catalogMagic {
		return bad("bad magic")
	}
	if b[4] != catalogVersion {
		return bad(fmt.Sprintf("unsupported version %d", b[4]))
	}
	b = b[5:]
	n, k := binary.Uvarint(b)
	if k <= 0 || n > 256 {
		return bad("bad count")
	}
	b = b[k:]
	out := make([]Collection, 0, n)
	for i := uint64(0); i < n; i++ {
		if len(b) < 1 {
			return bad("truncated entry")
		}
		prefix := b[0]
		l, k := binary.Uvarint(b[1:])
		if k <= 0 || l > maxCollectionName {
			return bad("bad name length")
		}
		b = b[1+k:]
		if uint64(len(b)) < l+8 {
			return bad("truncated name")
		}
		name := string(b[:l])
		created := int64(binary.LittleEndian.Uint64(b[l : l+8]))
		b = b[l+8:]
		out = append(out, Collection{Name: name, Prefix: prefix, CreatedAt: created})
	}
	if len(b) != 0 {
		return bad("trailing bytes")
	}
	return out, nil
}
