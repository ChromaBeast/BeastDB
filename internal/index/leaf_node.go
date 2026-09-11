package index

import (
	"encoding/binary"
	"sort"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

const (
	LeafEntrySize  = 18
	MaxLeafEntries = 200
)

// LeafNode manages sorted (Key, RID) entries on an on-disk 4KB page.
type LeafNode struct {
	data []byte
}

// InitLeafNode initializes a 4KB page buffer as an empty leaf node.
func InitLeafNode(data []byte, isRoot bool, parentID uint64) *LeafNode {
	h := NodeHeader{
		NodeType:     NodeTypeLeaf,
		IsRoot:       isRoot,
		KeyCount:     0,
		ParentPageID: parentID,
		NextPageID:   0,
	}
	WriteNodeHeader(data, h)
	return &LeafNode{data: data}
}

// AsLeafNode wraps existing page bytes into a LeafNode manager.
func AsLeafNode(data []byte) *LeafNode {
	return &LeafNode{data: data}
}

func (l *LeafNode) Header() NodeHeader {
	return ReadNodeHeader(l.data)
}

func (l *LeafNode) SetHeader(h NodeHeader) {
	WriteNodeHeader(l.data, h)
}

func (l *LeafNode) Key(i int) uint64 {
	offset := NodeHeaderSize + i*LeafEntrySize
	return binary.LittleEndian.Uint64(l.data[offset : offset+8])
}

func (l *LeafNode) SetKey(i int, key uint64) {
	offset := NodeHeaderSize + i*LeafEntrySize
	binary.LittleEndian.PutUint64(l.data[offset:offset+8], key)
}

func (l *LeafNode) RID(i int) storage.RID {
	offset := NodeHeaderSize + i*LeafEntrySize + 8
	return storage.RID{
		PageID: binary.LittleEndian.Uint64(l.data[offset : offset+8]),
		SlotID: binary.LittleEndian.Uint16(l.data[offset+8 : offset+10]),
	}
}

func (l *LeafNode) SetRID(i int, rid storage.RID) {
	offset := NodeHeaderSize + i*LeafEntrySize + 8
	binary.LittleEndian.PutUint64(l.data[offset:offset+8], rid.PageID)
	binary.LittleEndian.PutUint16(l.data[offset+8:offset+10], rid.SlotID)
}

// Lookup performs binary search for key and returns the matching RID.
func (l *LeafNode) Lookup(key uint64) (storage.RID, bool) {
	h := l.Header()
	idx := sort.Search(int(h.KeyCount), func(i int) bool {
		return l.Key(i) >= key
	})
	if idx < int(h.KeyCount) && l.Key(idx) == key {
		return l.RID(idx), true
	}
	return storage.RID{}, false
}

// Insert adds or updates a (Key, RID) pair in sorted order.
func (l *LeafNode) Insert(key uint64, rid storage.RID) bool {
	h := l.Header()
	idx := sort.Search(int(h.KeyCount), func(i int) bool {
		return l.Key(i) >= key
	})

	if idx < int(h.KeyCount) && l.Key(idx) == key {
		l.SetRID(idx, rid)
		return false
	}

	for i := int(h.KeyCount); i > idx; i-- {
		l.SetKey(i, l.Key(i-1))
		l.SetRID(i, l.RID(i-1))
	}
	l.SetKey(idx, key)
	l.SetRID(idx, rid)

	h.KeyCount++
	l.SetHeader(h)
	return true
}

// IsFull reports whether the leaf has reached maximum capacity.
func (l *LeafNode) IsFull() bool {
	return l.Header().KeyCount >= MaxLeafEntries
}

// Split moves the upper half of keys to rightNode, linking pages together.
func (l *LeafNode) Split(rightData []byte, newPageID uint64) (uint64, *LeafNode) {
	h := l.Header()
	mid := int(h.KeyCount) / 2
	rightCount := int(h.KeyCount) - mid

	rightNode := InitLeafNode(rightData, false, h.ParentPageID)
	rightH := rightNode.Header()
	rightH.NextPageID = h.NextPageID
	rightH.KeyCount = uint16(rightCount)

	for i := 0; i < rightCount; i++ {
		rightNode.SetKey(i, l.Key(mid+i))
		rightNode.SetRID(i, l.RID(mid+i))
	}
	rightNode.SetHeader(rightH)

	h.KeyCount = uint16(mid)
	h.NextPageID = newPageID
	l.SetHeader(h)

	splitKey := rightNode.Key(0)
	return splitKey, rightNode
}
