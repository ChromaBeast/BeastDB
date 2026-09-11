package index

import (
	"github.com/ChromaBeast/beastdb/internal/storage"
)

// splitLeafAndInsert splits an overflowing leaf page and inserts (key, rid).
func (t *BPlusTree) splitLeafAndInsert(path []uint64, leafID uint64, leafPage *storage.SlottedPage, key uint64, rid storage.RID) error {
	newPage, newPageID, err := t.bpm.NewPage()
	if err != nil {
		_ = t.bpm.UnpinPage(leafID, false)
		return err
	}

	leaf := AsLeafNode(leafPage.Data())
	splitKey, rightLeaf := leaf.Split(newPage.Data(), newPageID)

	if key < splitKey {
		leaf.Insert(key, rid)
	} else {
		rightLeaf.Insert(key, rid)
	}

	if leafID == t.rootPageID {
		err = t.createNewRoot(leafID, splitKey, newPageID)
		_ = t.bpm.UnpinPage(leafID, true)
		_ = t.bpm.UnpinPage(newPageID, true)
		return err
	}

	_ = t.bpm.UnpinPage(leafID, true)
	_ = t.bpm.UnpinPage(newPageID, true)

	return t.insertIntoParent(path, splitKey, newPageID)
}

// createNewRoot allocates an internal node to become the new root of the tree.
func (t *BPlusTree) createNewRoot(leftChildID uint64, splitKey uint64, rightChildID uint64) error {
	newRootPage, newRootID, err := t.bpm.NewPage()
	if err != nil {
		return err
	}

	InitInternalNode(newRootPage.Data(), true, 0, leftChildID)
	rootInternal := AsInternalNode(newRootPage.Data())
	rootInternal.Insert(splitKey, rightChildID)

	t.setParent(leftChildID, newRootID, false)
	t.setParent(rightChildID, newRootID, false)

	t.rootPageID = newRootID
	return t.bpm.UnpinPage(newRootID, true)
}

// setParent updates the ParentPageID and IsRoot metadata of a node.
func (t *BPlusTree) setParent(childID uint64, parentID uint64, isRoot bool) {
	page, err := t.bpm.FetchPage(childID)
	if err != nil {
		return
	}
	h := ReadNodeHeader(page.Data())
	h.ParentPageID = parentID
	h.IsRoot = isRoot
	WriteNodeHeader(page.Data(), h)
	_ = t.bpm.UnpinPage(childID, true)
}

// insertIntoParent propagates node splits up the ancestor path.
func (t *BPlusTree) insertIntoParent(path []uint64, key uint64, childID uint64) error {
	if len(path) == 0 {
		return nil
	}

	parentID := path[len(path)-1]
	path = path[:len(path)-1]

	parentPage, err := t.bpm.FetchPage(parentID)
	if err != nil {
		return err
	}

	parent := AsInternalNode(parentPage.Data())
	t.setParent(childID, parentID, false)

	if !parent.IsFull() {
		parent.Insert(key, childID)
		return t.bpm.UnpinPage(parentID, true)
	}

	newPage, newPageID, err := t.bpm.NewPage()
	if err != nil {
		_ = t.bpm.UnpinPage(parentID, false)
		return err
	}

	promotedKey, rightInternal := parent.Split(newPage.Data(), newPageID)
	if key < promotedKey {
		parent.Insert(key, childID)
	} else {
		rightInternal.Insert(key, childID)
		t.setParent(childID, newPageID, false)
	}

	_ = t.updateChildrenParent(rightInternal, newPageID)

	if parentID == t.rootPageID {
		err = t.createNewRoot(parentID, promotedKey, newPageID)
		_ = t.bpm.UnpinPage(parentID, true)
		_ = t.bpm.UnpinPage(newPageID, true)
		return err
	}

	_ = t.bpm.UnpinPage(parentID, true)
	_ = t.bpm.UnpinPage(newPageID, true)

	return t.insertIntoParent(path, promotedKey, newPageID)
}

// updateChildrenParent sets the ParentPageID for all children of an internal node.
func (t *BPlusTree) updateChildrenParent(internal *InternalNode, parentID uint64) error {
	h := internal.Header()
	t.setParent(internal.Child0(), parentID, false)
	for i := 0; i < int(h.KeyCount); i++ {
		t.setParent(internal.Child(i), parentID, false)
	}
	return nil
}
