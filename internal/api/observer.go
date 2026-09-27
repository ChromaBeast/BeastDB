package api

// CommitObserver defines a listener for durable write events committed to WAL.
// Implementations can handle asynchronous or synchronous replication fan-out.
type CommitObserver interface {
	OnCommit(lsn uint64, opType byte, key, value []byte)
}

// SetCommitObserver registers a listener to be notified of committed mutations.
func (e *Engine) SetCommitObserver(obs CommitObserver) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observer = obs
}

// notifyCommit alerts the registered observer if present.
func (e *Engine) notifyCommit(lsn uint64, opType byte, key, value []byte) {
	if e.observer != nil {
		e.observer.OnCommit(lsn, opType, key, value)
	}
}
