package index

import (
	"github.com/ChromaBeast/beastdb/internal/storage"
)

const (
	MinLeafEntries = MaxLeafEntries / 2
)

// Delete removes key from the B+ Tree, rebalancing via borrowing or leaf merging.
func (t *BPlusTree) Delete(key uint64) error {
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
			if !leaf.Delete(key) {
				_ = t.bpm.UnpinPage(currPageID, false)
				return ErrKeyNotFound
			}

			// Root leaf never underflows
			if currPageID == t.rootPageID {
				return t.bpm.UnpinPage(currPageID, true)
			}

			if int(leaf.Header().KeyCount) >= MinLeafEntries {
				return t.bpm.UnpinPage(currPageID, true)
			}

			return t.handleLeafUnderflow(path, currPageID, page)
		}

		internal := AsInternalNode(page.Data())
		nextID := internal.Lookup(key)
		path = append(path, currPageID)
		_ = t.bpm.UnpinPage(currPageID, false)
		currPageID = nextID
	}
}

// handleLeafUnderflow borrows from right sibling or merges leaves together.
func (t *BPlusTree) handleLeafUnderflow(path []uint64, leafID uint64, leafPage *storage.SlottedPage) error {
	if len(path) == 0 {
		return t.bpm.UnpinPage(leafID, true)
	}

	parentID := path[len(path)-1]
	parentPage, err := t.bpm.FetchPage(parentID)
	if err != nil {
		return t.bpm.UnpinPage(leafID, true)
	}
	parent := AsInternalNode(parentPage.Data())

	leaf := AsLeafNode(leafPage.Data())
	rightID := leaf.Header().NextPageID

	// If right sibling exists, attempt borrow or merge
	if rightID != 0 {
		rightPage, err := t.bpm.FetchPage(rightID)
		if err == nil {
			rightLeaf := AsLeafNode(rightPage.Data())
			// 1. Borrow from right sibling
			if int(rightLeaf.Header().KeyCount) > MinLeafEntries {
				borrowKey := rightLeaf.Key(0)
				borrowRID := rightLeaf.RID(0)
				leaf.Insert(borrowKey, borrowRID)
				rightLeaf.Delete(borrowKey)

				t.updateParentKey(parent, rightID, rightLeaf.Key(0))
				_ = t.bpm.UnpinPage(parentID, true)
				_ = t.bpm.UnpinPage(rightID, true)
				return t.bpm.UnpinPage(leafID, true)
			}

			// 2. Merge right sibling into leaf
			for i := 0; i < int(rightLeaf.Header().KeyCount); i++ {
				leaf.Insert(rightLeaf.Key(i), rightLeaf.RID(i))
			}
			leafH := leaf.Header()
			leafH.NextPageID = rightLeaf.Header().NextPageID
			leaf.SetHeader(leafH)

			parent.Delete(rightLeaf.Key(0))
			_ = t.bpm.UnpinPage(rightID, true)
			_ = t.bpm.UnpinPage(parentID, true)
			return t.bpm.UnpinPage(leafID, true)
		}
	}

	_ = t.bpm.UnpinPage(parentID, false)
	return t.bpm.UnpinPage(leafID, true)
}

// updateParentKey updates the routing key corresponding to a child page.
func (t *BPlusTree) updateParentKey(parent *InternalNode, childID uint64, newKey uint64) {
	h := parent.Header()
	for i := 0; i < int(h.KeyCount); i++ {
		if parent.Child(i) == childID {
			parent.SetKey(i, newKey)
			return
		}
	}
}
