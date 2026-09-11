package index

import (
	"encoding/binary"
	"sort"
)

const (
	InternalHeaderOffset = NodeHeaderSize + 8 // 20B Header + 8B Child0 = 28B
	InternalEntrySize    = 16                 // Key (8B) + ChildPageID (8B)
	MaxInternalKeys      = 200
)

// InternalNode manages routing keys and child page pointers on an on-disk 4KB page.
type InternalNode struct {
	data []byte
}

// InitInternalNode initializes a 4KB page buffer as an internal router node.
func InitInternalNode(data []byte, isRoot bool, parentID uint64, child0 uint64) *InternalNode {
	h := NodeHeader{
		NodeType:     NodeTypeInternal,
		IsRoot:       isRoot,
		KeyCount:     0,
		ParentPageID: parentID,
		NextPageID:   0,
	}
	WriteNodeHeader(data, h)
	binary.LittleEndian.PutUint64(data[NodeHeaderSize:NodeHeaderSize+8], child0)
	return &InternalNode{data: data}
}

// AsInternalNode wraps an existing page slice into an InternalNode router.
func AsInternalNode(data []byte) *InternalNode {
	return &InternalNode{data: data}
}

func (in *InternalNode) Header() NodeHeader {
	return ReadNodeHeader(in.data)
}

func (in *InternalNode) SetHeader(h NodeHeader) {
	WriteNodeHeader(in.data, h)
}

func (in *InternalNode) Child0() uint64 {
	return binary.LittleEndian.Uint64(in.data[NodeHeaderSize : NodeHeaderSize+8])
}

func (in *InternalNode) SetChild0(child0 uint64) {
	binary.LittleEndian.PutUint64(in.data[NodeHeaderSize:NodeHeaderSize+8], child0)
}

func (in *InternalNode) Key(i int) uint64 {
	offset := InternalHeaderOffset + i*InternalEntrySize
	return binary.LittleEndian.Uint64(in.data[offset : offset+8])
}

func (in *InternalNode) SetKey(i int, key uint64) {
	offset := InternalHeaderOffset + i*InternalEntrySize
	binary.LittleEndian.PutUint64(in.data[offset:offset+8], key)
}

func (in *InternalNode) Child(i int) uint64 {
	offset := InternalHeaderOffset + i*InternalEntrySize + 8
	return binary.LittleEndian.Uint64(in.data[offset : offset+8])
}

func (in *InternalNode) SetChild(i int, childID uint64) {
	offset := InternalHeaderOffset + i*InternalEntrySize + 8
	binary.LittleEndian.PutUint64(in.data[offset:offset+8], childID)
}

// Lookup binary-searches routing keys to select which child page ID to descend into.
func (in *InternalNode) Lookup(key uint64) uint64 {
	h := in.Header()
	idx := sort.Search(int(h.KeyCount), func(i int) bool {
		return key < in.Key(i)
	})
	if idx == 0 {
		return in.Child0()
	}
	return in.Child(idx - 1)
}

// Insert adds a routing key and its right-hand child page ID in sorted position.
func (in *InternalNode) Insert(key uint64, childPageID uint64) {
	h := in.Header()
	idx := sort.Search(int(h.KeyCount), func(i int) bool {
		return in.Key(i) >= key
	})

	for i := int(h.KeyCount); i > idx; i-- {
		in.SetKey(i, in.Key(i-1))
		in.SetChild(i, in.Child(i-1))
	}
	in.SetKey(idx, key)
	in.SetChild(idx, childPageID)

	h.KeyCount++
	in.SetHeader(h)
}

// IsFull reports whether the internal node has reached maximum routing capacity.
func (in *InternalNode) IsFull() bool {
	return in.Header().KeyCount >= MaxInternalKeys
}

// Split moves the upper half of keys to a new internal node and returns the promoted key.
func (in *InternalNode) Split(rightData []byte, newPageID uint64) (uint64, *InternalNode) {
	h := in.Header()
	mid := int(h.KeyCount) / 2
	promotedKey := in.Key(mid)

	rightChild0 := in.Child(mid)
	rightNode := InitInternalNode(rightData, false, h.ParentPageID, rightChild0)
	rightH := rightNode.Header()

	rightCount := int(h.KeyCount) - mid - 1
	rightH.KeyCount = uint16(rightCount)

	for i := 0; i < rightCount; i++ {
		rightNode.SetKey(i, in.Key(mid+1+i))
		rightNode.SetChild(i, in.Child(mid+1+i))
	}
	rightNode.SetHeader(rightH)

	h.KeyCount = uint16(mid)
	in.SetHeader(h)

	return promotedKey, rightNode
}
