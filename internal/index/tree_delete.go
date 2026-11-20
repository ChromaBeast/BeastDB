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

			// Root leaf never underflows.
			if currPageID == t.rootPageID {
				return t.bpm.UnpinPage(currPageID, true)
			}

			if int(leaf.Header().KeyCount) >= MinLeafEntries {
				return t.bpm.UnpinPage(currPageID, true)
			}

			t.epoch.Add(1) // structural change: merge or borrow
			return t.handleLeafUnderflow(path, currPageID, page)
		}

		internal := AsInternalNode(page.Data())
		nextID := internal.Lookup(key)
		path = append(path, currPageID)
		_ = t.bpm.UnpinPage(currPageID, false)
		currPageID = nextID
	}
}

// handleLeafUnderflow borrows from left or right sibling, or merges.
// Fix #3: We now try the LEFT sibling first (via parent lookup), then the right.
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

	// --- Try LEFT sibling first (found via parent child-pointer scan) ---
	leftID := leftSiblingID(parent, leafID)
	if leftID != 0 {
		leftPage, err := t.bpm.FetchPage(leftID)
		if err == nil {
			leftLeaf := AsLeafNode(leftPage.Data())

			if int(leftLeaf.Header().KeyCount) > MinLeafEntries {
				// Borrow last key from left sibling.
				lastIdx := int(leftLeaf.Header().KeyCount) - 1
				borrowKey := leftLeaf.Key(lastIdx)
				borrowRID := leftLeaf.RID(lastIdx)
				leaf.Insert(borrowKey, borrowRID)
				leftLeaf.Delete(borrowKey)

				t.updateParentKey(parent, leafID, leaf.Key(0))
				_ = t.bpm.UnpinPage(parentID, true)
				_ = t.bpm.UnpinPage(leftID, true)
				return t.bpm.UnpinPage(leafID, true)
			}

			// Merge leaf into left sibling (left absorbs current leaf).
			for i := 0; i < int(leaf.Header().KeyCount); i++ {
				leftLeaf.Insert(leaf.Key(i), leaf.RID(i))
			}
			leftH := leftLeaf.Header()
			leftH.NextPageID = leaf.Header().NextPageID
			leftLeaf.SetHeader(leftH)

			// Drop the parent's entry for the absorbed leaf by child pointer:
			// its separator stops matching the leaf's first key as soon as
			// that key is deleted.
			parent.DeleteChild(leafID)
			_ = t.bpm.UnpinPage(leftID, true)
			_ = t.bpm.UnpinPage(leafID, true)
			t.freePage(leafID)
			return t.afterChildRemoved(path, parentID, parent)
		}
	}

	// --- Fall back to RIGHT sibling ---
	// Only a sibling under the same parent qualifies: the next leaf in the
	// chain may belong to a different parent, whose separators this code
	// cannot reach.
	rightID := rightSiblingID(parent, leafID)
	if rightID != 0 {
		rightPage, err := t.bpm.FetchPage(rightID)
		if err == nil {
			rightLeaf := AsLeafNode(rightPage.Data())

			if int(rightLeaf.Header().KeyCount) > MinLeafEntries {
				// Borrow first key from right sibling.
				borrowKey := rightLeaf.Key(0)
				borrowRID := rightLeaf.RID(0)
				leaf.Insert(borrowKey, borrowRID)
				rightLeaf.Delete(borrowKey)

				t.updateParentKey(parent, rightID, rightLeaf.Key(0))
				_ = t.bpm.UnpinPage(parentID, true)
				_ = t.bpm.UnpinPage(rightID, true)
				return t.bpm.UnpinPage(leafID, true)
			}

			// Merge right sibling into current leaf.
			for i := 0; i < int(rightLeaf.Header().KeyCount); i++ {
				leaf.Insert(rightLeaf.Key(i), rightLeaf.RID(i))
			}
			leafH := leaf.Header()
			leafH.NextPageID = rightLeaf.Header().NextPageID
			leaf.SetHeader(leafH)

			parent.DeleteChild(rightID)
			_ = t.bpm.UnpinPage(rightID, true)
			_ = t.bpm.UnpinPage(leafID, true)
			t.freePage(rightID)
			return t.afterChildRemoved(path, parentID, parent)
		}
	}

	_ = t.bpm.UnpinPage(parentID, false)
	return t.bpm.UnpinPage(leafID, true)
}

// leftSiblingID scans the parent's child pointer array to find the left sibling.
// Child layout: [Child0] [Key0,Child(0)] [Key1,Child(1)] ... [Keyn,Child(n)]
func leftSiblingID(parent *InternalNode, leafID uint64) uint64 {
	h := parent.Header()
	if parent.Child0() == leafID {
		return 0 // leftmost child has no left sibling
	}
	// Child(0) is the right child of Key(0); its left sibling is Child0.
	if parent.Child(0) == leafID {
		return parent.Child0()
	}
	for i := 1; i < int(h.KeyCount); i++ {
		if parent.Child(i) == leafID {
			return parent.Child(i - 1)
		}
	}
	return 0
}

// afterChildRemoved runs after a merge removed one of parent's routing
// entries; parent is pinned and is unpinned here. An internal node left with
// no keys routes everything to its only child, so it is spliced out: a root
// hands the root role to that child, any other node is replaced by the child
// in the grandparent. Either way its page is freed.
func (t *BPlusTree) afterChildRemoved(path []uint64, parentID uint64, parent *InternalNode) error {
	if parent.Header().KeyCount > 0 {
		return t.bpm.UnpinPage(parentID, true)
	}
	child := parent.Child0()
	if err := t.bpm.UnpinPage(parentID, true); err != nil {
		return err
	}

	if parentID == t.rootPageID {
		t.setParent(child, 0, true)
		t.rootPageID = child
		if t.onRootChange != nil {
			t.onRootChange(child)
		}
		t.freePage(parentID)
		return nil
	}
	if len(path) < 2 {
		return nil // no grandparent on record; keep the empty node, which still routes correctly
	}
	grandID := path[len(path)-2]
	grandPage, err := t.bpm.FetchPage(grandID)
	if err != nil {
		return nil // same: leaving the node in place is safe
	}
	grand := AsInternalNode(grandPage.Data())
	if !grand.ReplaceChild(parentID, child) {
		_ = t.bpm.UnpinPage(grandID, false)
		return nil
	}
	_ = t.bpm.UnpinPage(grandID, true)
	t.setParent(child, grandID, false)
	t.freePage(parentID)
	return nil
}

// freePage returns a page the tree no longer references to the free list.
// A page that cannot be freed (still pinned by a reader) is left allocated:
// a leaked page is harmless.
func (t *BPlusTree) freePage(id uint64) {
	_ = t.bpm.FreePage(id)
}

// rightSiblingID returns the child that follows leafID in the parent, or 0.
func rightSiblingID(parent *InternalNode, leafID uint64) uint64 {
	n := int(parent.Header().KeyCount)
	if n == 0 {
		return 0
	}
	if parent.Child0() == leafID {
		return parent.Child(0)
	}
	for i := 0; i < n-1; i++ {
		if parent.Child(i) == leafID {
			return parent.Child(i + 1)
		}
	}
	return 0
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
