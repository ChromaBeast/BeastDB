package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// A chain stores a byte string of any length in a linked list of pages taken
// from the buffer pool. The engine uses chains for structures it saves at a
// checkpoint, such as the free-space map. Each page holds:
//
//	magic "BDBCHN01" | kind (1) | next page (8) | used bytes (2) | payload
var chainMagic = []byte("BDBCHN01")

const (
	chainHeader   = 8 + 1 + 8 + 2
	chainCapacity = PageDataEnd - chainHeader
)

// ErrBadChain reports a chain page that is missing, of the wrong kind or
// malformed.
var ErrBadChain = errors.New("storage: malformed page chain")

// WriteChain stores data in new pages and returns the first page ID (0 for
// empty data) and every page used, in order. At most two pages are pinned at
// a time, so a chain can be far larger than the buffer pool.
func WriteChain(bpm *BufferPoolManager, kind byte, data []byte) (uint64, []uint64, error) {
	if len(data) == 0 {
		return 0, nil, nil
	}
	n := (len(data) + chainCapacity - 1) / chainCapacity
	ids := make([]uint64, 0, n)
	page, id, err := bpm.NewPage()
	if err != nil {
		return 0, nil, err
	}
	ids = append(ids, id)
	for i := 0; i < n; i++ {
		var next uint64
		var nextPage *SlottedPage
		if i+1 < n {
			if nextPage, next, err = bpm.NewPage(); err != nil {
				_ = bpm.UnpinPage(id, true)
				FreeChain(bpm, ids)
				return 0, nil, err
			}
			ids = append(ids, next)
		}
		buf := page.Data()
		clear(buf[:PageDataEnd])
		chunk := data[i*chainCapacity : min((i+1)*chainCapacity, len(data))]
		copy(buf[:8], chainMagic)
		buf[8] = kind
		binary.LittleEndian.PutUint64(buf[9:17], next)
		binary.LittleEndian.PutUint16(buf[17:19], uint16(len(chunk)))
		copy(buf[chainHeader:], chunk)
		_ = bpm.UnpinPage(id, true)
		page, id = nextPage, next
	}
	return ids[0], ids, nil
}

// ReadChain returns the data stored from head and the pages that hold it.
// Head 0 is an empty chain: page 0 is the meta page and never part of one.
func ReadChain(bpm *BufferPoolManager, kind byte, head uint64) ([]byte, []uint64, error) {
	var out []byte
	var ids []uint64
	limit := bpm.disk.NumPages()
	for id := head; id != 0; {
		if id >= limit || uint64(len(ids)) >= limit {
			return nil, ids, ErrBadChain // past the file, or a cycle
		}
		p, err := bpm.FetchPage(id)
		if err != nil {
			return nil, ids, err
		}
		buf := p.Data()
		used := int(binary.LittleEndian.Uint16(buf[17:19]))
		ok := bytes.Equal(buf[:8], chainMagic) && buf[8] == kind && used <= chainCapacity
		var next uint64
		if ok {
			out = append(out, buf[chainHeader:chainHeader+used]...)
			next = binary.LittleEndian.Uint64(buf[9:17])
		}
		_ = bpm.UnpinPage(id, false)
		if !ok {
			return nil, ids, ErrBadChain
		}
		ids = append(ids, id)
		id = next
	}
	return out, ids, nil
}

// FreeChain returns a chain's pages to the free list.
func FreeChain(bpm *BufferPoolManager, ids []uint64) {
	for _, id := range ids {
		_ = bpm.FreePage(id)
	}
}
