// Package simfile is an in-memory file for crash tests. It tracks what Sync has
// made durable, so a test can simulate power loss: Crash returns the last
// synced image plus an arbitrary subset of the writes made since, with the file
// size either persisted or not. That is the window a real OS leaves open when
// writes are not fsynced, and the one the engine's ordering rules must survive.
package simfile

import (
	"errors"
	"io"
	"io/fs"
	"math/rand"
	"sync"
	"time"
)

// ErrInjected is returned by WriteAt once the budget set by FailWritesAfter runs out.
var ErrInjected = errors.New("simfile: injected write failure")

type op struct {
	off      int64
	data     []byte // nil for a truncate
	truncate bool
	size     int64
}

// File implements the subset of *os.File the storage layer uses.
type File struct {
	mu        sync.Mutex
	name      string
	data      []byte // what reads see now
	durable   []byte // image as of the last Sync
	pending   []op   // writes and truncates since the last Sync
	syncs     int
	failAfter int // WriteAt calls left before ErrInjected; <0 disables
}

// New returns an empty file.
func New(name string) *File { return &File{name: name, failAfter: -1} }

func (f *File) Name() string { return f.name }

func (f *File) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *File) WriteAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAfter == 0 {
		return 0, ErrInjected
	}
	if f.failAfter > 0 {
		f.failAfter--
	}
	if end := off + int64(len(p)); end > int64(len(f.data)) {
		f.data = resize(f.data, end)
	}
	copy(f.data[off:], p)
	f.pending = append(f.pending, op{off: off, data: append([]byte(nil), p...)})
	return len(p), nil
}

func (f *File) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = resize(f.data, size)
	f.pending = append(f.pending, op{truncate: true, size: size})
	return nil
}

func (f *File) Sync() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.durable = append(f.durable[:0:0], f.data...)
	f.pending = nil
	f.syncs++
	return nil
}

func (f *File) Close() error { return nil }

func (f *File) Stat() (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return info{name: f.name, size: int64(len(f.data))}, nil
}

// Syncs reports how many times Sync has been called.
func (f *File) Syncs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.syncs
}

// FailWritesAfter makes WriteAt fail with ErrInjected after n more successful calls.
// A negative n turns injection off.
func (f *File) FailWritesAfter(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAfter = n
}

// Crash simulates power loss and returns the file as a restart would find it.
// Starting from the last synced image, each unsynced write or truncate survives
// with probability keep (0 keeps none, 1 keeps all), applied in order. The file
// size then lands on either the synced size or the size reached by the kept
// operations, chosen at random, because the OS may persist data blocks and
// size metadata independently.
func (f *File) Crash(rng *rand.Rand, keep float64) *File {
	f.mu.Lock()
	defer f.mu.Unlock()
	img := append([]byte(nil), f.durable...)
	for _, o := range f.pending {
		if rng.Float64() >= keep {
			continue
		}
		if o.truncate {
			img = resize(img, o.size)
			continue
		}
		if end := o.off + int64(len(o.data)); end > int64(len(img)) {
			img = resize(img, end)
		}
		copy(img[o.off:], o.data)
	}
	if rng.Intn(2) == 0 {
		img = resize(img, int64(len(f.durable)))
	}
	return &File{name: f.name, data: img, durable: append([]byte(nil), img...), failAfter: -1}
}

func resize(b []byte, size int64) []byte {
	if size <= int64(len(b)) {
		return b[:size]
	}
	return append(b, make([]byte, size-int64(len(b)))...)
}

type info struct {
	name string
	size int64
}

func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return i.size }
func (i info) Mode() fs.FileMode  { return 0o600 }
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return false }
func (i info) Sys() any           { return nil }
