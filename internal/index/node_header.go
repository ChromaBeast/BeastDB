package index

import (
	"encoding/binary"
)

const (
	// NodeType identifiers
	NodeTypeLeaf     uint8 = 1
	NodeTypeInternal uint8 = 2

	// NodeHeaderSize is 20 bytes: Type(1B) + IsRoot(1B) + KeyCount(2B) + Parent(8B) + Next(8B)
	NodeHeaderSize = 20
)

// NodeHeader encapsulates common metadata for all on-disk B+ Tree nodes.
type NodeHeader struct {
	NodeType     uint8
	IsRoot       bool
	KeyCount     uint16
	ParentPageID uint64
	NextPageID   uint64
}

// ReadNodeHeader parses node metadata from the first 20 bytes of a page buffer.
func ReadNodeHeader(data []byte) NodeHeader {
	var isRoot bool
	if data[1] == 1 {
		isRoot = true
	}

	return NodeHeader{
		NodeType:     data[0],
		IsRoot:       isRoot,
		KeyCount:     binary.LittleEndian.Uint16(data[2:4]),
		ParentPageID: binary.LittleEndian.Uint64(data[4:12]),
		NextPageID:   binary.LittleEndian.Uint64(data[12:20]),
	}
}

// WriteNodeHeader serializes node metadata directly into the page buffer.
func WriteNodeHeader(data []byte, h NodeHeader) {
	data[0] = h.NodeType
	if h.IsRoot {
		data[1] = 1
	} else {
		data[1] = 0
	}
	binary.LittleEndian.PutUint16(data[2:4], h.KeyCount)
	binary.LittleEndian.PutUint64(data[4:12], h.ParentPageID)
	binary.LittleEndian.PutUint64(data[12:20], h.NextPageID)
}
