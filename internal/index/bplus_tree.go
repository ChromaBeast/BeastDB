package index

import (
	"errors"
	"sync"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

var (
	ErrKeyNotFound = errors.New("bplus: key not found in index")
)

// BPlusTree coordinates on-disk B+ Tree operations via the BufferPoolManager.
type BPlusTree struct {
	rootPageID uint64
	bpm        *storage.BufferPoolManager
	mu         sync.RWMutex
}

// CreateBPlusTree initializes a new B+ Tree with an empty root leaf node.
func CreateBPlusTree(bpm *storage.BufferPoolManager) (*BPlusTree, error) {
	rootPage, rootID, err := bpm.NewPage()
	if err != nil {
		return nil, err
	}

	InitLeafNode(rootPage.Data(), true, 0)
	if err := bpm.UnpinPage(rootID, true); err != nil {
		return nil, err
	}

	return &BPlusTree{
		rootPageID: rootID,
		bpm:        bpm,
	}, nil
}

// OpenBPlusTree wraps an existing B+ Tree rooted at rootPageID.
func OpenBPlusTree(rootPageID uint64, bpm *storage.BufferPoolManager) *BPlusTree {
	return &BPlusTree{
		rootPageID: rootPageID,
		bpm:        bpm,
	}
}

// RootPageID returns the physical page ID of the tree root.
func (t *BPlusTree) RootPageID() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rootPageID
}

// Find binary-searches the B+ Tree for key and returns its physical RID.
func (t *BPlusTree) Find(key uint64) (storage.RID, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	currPageID := t.rootPageID
	for {
		page, err := t.bpm.FetchPage(currPageID)
		if err != nil {
			return storage.RID{}, err
		}

		h := ReadNodeHeader(page.Data())
		if h.NodeType == NodeTypeLeaf {
			leaf := AsLeafNode(page.Data())
			rid, found := leaf.Lookup(key)
			_ = t.bpm.UnpinPage(currPageID, false)
			if !found {
				return storage.RID{}, ErrKeyNotFound
			}
			return rid, nil
		}

		internal := AsInternalNode(page.Data())
		nextPageID := internal.Lookup(key)
		_ = t.bpm.UnpinPage(currPageID, false)
		currPageID = nextPageID
	}
}

// Insert adds a (Key, RID) pair into the B+ Tree, splitting nodes if necessary.
func (t *BPlusTree) Insert(key uint64, rid storage.RID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var path []uint64
	currPageID := t.rootPageID

	for {
		page, err := t.bpm.FetchPage(currPageID)
		if err != nil {
			return err
		}

		h := ReadNodeHeader(page.Data())
		if h.NodeType == NodeTypeLeaf {
			leaf := AsLeafNode(page.Data())
			if !leaf.IsFull() {
				leaf.Insert(key, rid)
				return t.bpm.UnpinPage(currPageID, true)
			}
			return t.splitLeafAndInsert(path, currPageID, page, key, rid)
		}

		internal := AsInternalNode(page.Data())
		nextPageID := internal.Lookup(key)
		path = append(path, currPageID)
		_ = t.bpm.UnpinPage(currPageID, false)
		currPageID = nextPageID
	}
}
