package index

import (
	"sort"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// Cursor provides zero-allocation streaming range scans across B+ Tree leaf pages.
type Cursor struct {
	tree       *BPlusTree
	currPageID uint64
	currPage   *storage.SlottedPage
	slotIdx    int
	endKey     uint64
	isClosed   bool
}

// Scan initializes a streaming range scan from startKey up to endKey inclusive.
func (t *BPlusTree) Scan(startKey, endKey uint64) (*Cursor, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if startKey > endKey {
		return &Cursor{isClosed: true}, nil
	}

	currPageID := t.rootPageID
	for {
		page, err := t.bpm.FetchPage(currPageID)
		if err != nil {
			return nil, err
		}

		h := ReadNodeHeader(page.Data())
		if h.NodeType == NodeTypeLeaf {
			leaf := AsLeafNode(page.Data())
			idx := sort.Search(int(h.KeyCount), func(i int) bool {
				return leaf.Key(i) >= startKey
			})

			return &Cursor{
				tree:       t,
				currPageID: currPageID,
				currPage:   page,
				slotIdx:    idx,
				endKey:     endKey,
				isClosed:   false,
			}, nil
		}

		internal := AsInternalNode(page.Data())
		nextID := internal.Lookup(startKey)
		_ = t.bpm.UnpinPage(currPageID, false)
		currPageID = nextID
	}
}

// Next yields the next (Key, RID) pair in the scan via hand-over-hand page pinning.
func (c *Cursor) Next() (uint64, storage.RID, bool, error) {
	if c.isClosed {
		return 0, storage.RID{}, false, nil
	}

	for {
		leaf := AsLeafNode(c.currPage.Data())
		if c.slotIdx < int(leaf.Header().KeyCount) {
			key := leaf.Key(c.slotIdx)
			if key > c.endKey {
				_ = c.Close()
				return 0, storage.RID{}, false, nil
			}

			rid := leaf.RID(c.slotIdx)
			c.slotIdx++
			return key, rid, true, nil
		}

		nextID := leaf.Header().NextPageID
		if nextID == 0 {
			_ = c.Close()
			return 0, storage.RID{}, false, nil
		}

		// Hand-over-hand pinning: fetch next before releasing current
		nextPage, err := c.tree.bpm.FetchPage(nextID)
		if err != nil {
			_ = c.Close()
			return 0, storage.RID{}, false, err
		}

		_ = c.tree.bpm.UnpinPage(c.currPageID, false)
		c.currPageID = nextID
		c.currPage = nextPage
		c.slotIdx = 0
	}
}

// Close terminates the scan and safely unpins the active leaf frame.
func (c *Cursor) Close() error {
	if c.isClosed {
		return nil
	}
	c.isClosed = true

	if c.currPage != nil {
		err := c.tree.bpm.UnpinPage(c.currPageID, false)
		c.currPage = nil
		return err
	}
	return nil
}
