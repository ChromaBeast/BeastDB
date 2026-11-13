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
	// replicated marks a write applied from a leader's WAL; only client writes
	// are refused on a read-only engine.
	replicated bool
}

// committer is the group-commit queue. Writers enqueue a request; whichever
// writer finds no commit in progress becomes the leader and runs the queued
// group itself, on its own goroutine. This avoids a cross-goroutine wake-up
// for an uncontended write, which costs up to a timer tick on Windows. If more
// writers queued while the leader was busy, the leader hands the remaining work
// to a drain goroutine and returns to its caller.
//
// A group runs back to back under the exclusive engine lock (appending to the
// WAL without fsync), issues ONE fsync for the whole group, notifies the
// replication observer in LSN order, and only then releases the lock and
// acknowledges the writers. Because the lock is held across the fsync, readers
// and the observer never see a write before it is durable, exactly as with
// fsync-per-write; the gain is that N queued writers pay for one fsync.
// Only one group runs at a time, so queue order is LSN order.
type committer struct {
	mu      sync.Mutex
	queue   []*writeReq
	running bool // a leader or drain goroutine owns the queue
	closed  bool
	idle    *sync.Cond // signalled (on mu) when running becomes false
}

// commit runs fn through the group-commit pipeline and returns once its WAL
// record (if it wrote one) is durable. fn runs with e.mu held exclusively and
// must record observer events via e.queueNotify rather than calling the
// observer directly.
func (e *Engine) commit(fn func() error) error { return e.submit(fn, false) }

// commitReplicated is commit for writes replayed from a leader, which must
// still apply while the engine is read-only.
func (e *Engine) commitReplicated(fn func() error) error { return e.submit(fn, true) }

func (e *Engine) submit(fn func() error, replicated bool) error {
	c := &e.cq
	req := &writeReq{run: fn, done: make(chan struct{}), replicated: replicated}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrEngineClosed
	}
	c.queue = append(c.queue, req)
	lead := !c.running
	if lead {
		c.running = true
	}
	c.mu.Unlock()

	if lead {
		e.lead()
	}
	<-req.done
	return req.err
}

// takeBatch removes and returns the queued writes. When the queue is empty it
// clears running (releasing leadership) and returns nil.
func (c *committer) takeBatch() []*writeReq {
	c.mu.Lock()
	defer c.mu.Unlock()
	batch := c.queue
	c.queue = nil
	if len(batch) == 0 {
		c.running = false
		if c.idle != nil {
			c.idle.Broadcast()
		}
	}
	return batch
}

// lead runs one group (which includes the leader's own request) on the
// caller's goroutine. If more writers arrived meanwhile, a drain goroutine
// keeps serving them so the leader's caller is not delayed.
func (e *Engine) lead() {
	c := &e.cq
	e.runBatch(c.takeBatch())
	c.mu.Lock()
	more := len(c.queue) > 0
	if !more {
		c.running = false
		if c.idle != nil {
			c.idle.Broadcast()
		}
	}
	c.mu.Unlock()
	if more {
		go e.drain()
	}
}

// drain serves queued writes until the queue is empty, then releases leadership.
func (e *Engine) drain() {
	c := &e.cq
	for {
		batch := c.takeBatch()
		if len(batch) == 0 {
			return
		}
		e.runBatch(batch)
	}
}

// stopCommitter rejects new writes and waits for in-flight ones to finish.
func (e *Engine) stopCommitter() {
	c := &e.cq
	c.mu.Lock()
	c.closed = true
	if c.idle == nil {
		c.idle = sync.NewCond(&c.mu)
	}
	for c.running {
		c.idle.Wait()
	}
	c.mu.Unlock()
}

func (e *Engine) runBatch(batch []*writeReq) {
	e.mu.Lock()
	e.pending = e.pending[:0]
	wrote := false
	for _, r := range batch {
		// Checked under e.mu so a client write can never land after
		// SetReadOnly(true) returns: this is what makes fencing exact.
		if e.readOnly && !r.replicated {
			r.err = ErrReadOnlyReplica
			continue
		}
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
