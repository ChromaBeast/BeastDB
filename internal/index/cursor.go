package index

import (
	"errors"
	"sort"

	"github.com/ChromaBeast/beastdb/internal/storage"
)

// ErrConcurrentModification is returned by Next() when a write modified the
// tree structure after this cursor was opened (epoch mismatch).
var ErrConcurrentModification = errors.New("cursor: tree modified during scan")

// Cursor provides zero-allocation streaming range scans across B+ Tree leaf pages.
// Close() MUST be called to release the pinned page and any held lock.
type Cursor struct {
	tree       *BPlusTree
	currPageID uint64
	currPage   *storage.SlottedPage
	slotIdx    int
	endKey     uint64
	epoch      uint64       // tree epoch captured at scan start
	release    func()       // optional lock-release callback (set by Engine.Scan)
	isClosed   bool
}

// Scan initializes a streaming range scan from startKey up to endKey inclusive.
func (t *BPlusTree) Scan(startKey, endKey uint64) (*Cursor, error) {
	return t.scan(startKey, endKey, nil)
}

// ScanWithRelease initializes a range scan and stores a release function that
// is called on Close(). Used by Engine.Scan to hold a read-lock for scan lifetime.
func (t *BPlusTree) ScanWithRelease(startKey, endKey uint64, release func()) (*Cursor, error) {
	return t.scan(startKey, endKey, release)
}

func (t *BPlusTree) scan(startKey, endKey uint64, release func()) (*Cursor, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if startKey > endKey {
		if release != nil {
			release()
		}
		return &Cursor{isClosed: true}, nil
	}

	snapEpoch := t.epoch.Load()
	currPageID := t.rootPageID

	for {
		page, err := t.bpm.FetchPage(currPageID)
		if err != nil {
			if release != nil {
				release()
			}
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
				epoch:      snapEpoch,
				release:    release,
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
// Returns ErrConcurrentModification if a structural write occurred since Scan().
func (c *Cursor) Next() (uint64, storage.RID, bool, error) {
	if c.isClosed {
		return 0, storage.RID{}, false, nil
	}

	// Detect structural changes (splits/merges) that invalidate our scan path.
	if c.tree.epoch.Load() != c.epoch {
		_ = c.Close()
		return 0, storage.RID{}, false, ErrConcurrentModification
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

		// Hand-over-hand pinning: fetch next before releasing current.
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

// Close terminates the scan, unpins the active leaf frame, and calls the release hook.
func (c *Cursor) Close() error {
	if c.isClosed {
		return nil
	}
	c.isClosed = true

	var err error
	if c.currPage != nil {
		err = c.tree.bpm.UnpinPage(c.currPageID, false)
		c.currPage = nil
	}
	if c.release != nil {
		c.release()
		c.release = nil
	}
	return err
}
