package api

import (
	"errors"
	"sync"
)

// ErrEngineClosed is returned for writes submitted after Close.
var ErrEngineClosed = errors.New("engine: closed")

// pendingNotify is a commit event held back until its WAL record is fsynced.
type pendingNotify struct {
	lsn    uint64
	opType byte
	key    []byte
	value  []byte
}

type writeReq struct {
	run  func() error // executes under e.mu: check, WAL append (no fsync), apply
	err  error
	done chan struct{}
	ok   bool // run appended a WAL record (so it depends on the batch fsync)
}

// committer is the group-commit queue. A single goroutine drains it: each pass
// takes every queued write, runs them back to back under the exclusive engine
// lock (appending to the WAL without fsync), issues ONE fsync for the whole
// group, notifies the replication observer in LSN order, and only then
// releases the lock and acknowledges the writers. Because the lock is held
// across the fsync, readers and the observer never see a write before it is
// durable, exactly as with fsync-per-write; the gain is that N queued writers
// pay for one fsync instead of N.
type committer struct {
	mu      sync.Mutex
	queue   []*writeReq
	started bool
	closed  bool
	wake    chan struct{}
	done    chan struct{}
}

// commit runs fn through the group-commit pipeline and returns once its WAL
// record (if it wrote one) is durable. fn runs with e.mu held exclusively and
// must record observer events via e.queueNotify rather than calling the
// observer directly.
func (e *Engine) commit(fn func() error) error {
	c := &e.cq
	req := &writeReq{run: fn, done: make(chan struct{})}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrEngineClosed
	}
	if !c.started {
		c.started = true
		c.wake = make(chan struct{}, 1)
		c.done = make(chan struct{})
		go e.commitLoop()
	}
	c.queue = append(c.queue, req)
	c.mu.Unlock()

	select {
	case c.wake <- struct{}{}:
	default:
	}
	<-req.done
	return req.err
}

func (e *Engine) commitLoop() {
	c := &e.cq
	defer close(c.done)
	for {
		<-c.wake
		for {
			c.mu.Lock()
			batch := c.queue
			c.queue = nil
			closed := c.closed
			c.mu.Unlock()
			if len(batch) == 0 {
				if closed {
					return
				}
				break
			}
			e.runBatch(batch)
		}
	}
}

// stopCommitter drains outstanding writes and stops the commit goroutine.
func (e *Engine) stopCommitter() {
	c := &e.cq
	c.mu.Lock()
	c.closed = true
	started := c.started
	c.mu.Unlock()
	if started {
		select {
		case c.wake <- struct{}{}:
		default:
		}
		<-c.done
	}
}

func (e *Engine) runBatch(batch []*writeReq) {
	e.mu.Lock()
	e.pending = e.pending[:0]
	wrote := false
	for _, r := range batch {
		before := e.wal.CurrentLSN()
		r.err = r.run()
		// A request that appended a WAL record must wait for the group fsync,
		// even if applying it then failed (the record is still in the log).
		r.ok = e.wal.CurrentLSN() != before
		wrote = wrote || r.ok
	}

	if wrote {
		if syncErr := e.wal.SyncIfEnabled(); syncErr != nil {
			// The records may or may not be on disk and memory already reflects
			// them: fail-stop. Nothing in this group is acknowledged or replicated.
			herr := e.halt(syncErr)
			for _, r := range batch {
				if r.ok {
					r.err = herr
				}
			}
			e.pending = e.pending[:0]
		}
	}
	if e.observer != nil {
		for _, n := range e.pending {
			e.observer.OnCommit(n.lsn, n.opType, n.key, n.value)
		}
	}
	e.pending = e.pending[:0]
	e.mu.Unlock()

	for _, r := range batch {
		close(r.done)
	}
}

// queueNotify records a commit event to be delivered after the group fsync.
// Caller holds e.mu.
func (e *Engine) queueNotify(lsn uint64, opType byte, key, value []byte) {
	e.pending = append(e.pending, pendingNotify{lsn: lsn, opType: opType, key: key, value: value})
}
