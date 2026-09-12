package replication

import (
	"sync"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

// Broadcaster distributes real-time committed WAL records to connected follower streams.
type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[string]chan *beastv1.WALRecordMessage
}

// NewBroadcaster initializes an empty replication broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]chan *beastv1.WALRecordMessage),
	}
}

// Subscribe registers a replica stream channel and returns an unsubscription closure.
func (b *Broadcaster) Subscribe(replicaID string, bufSize int) (<-chan *beastv1.WALRecordMessage, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *beastv1.WALRecordMessage, bufSize)
	b.subscribers[replicaID] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers, replicaID)
		close(ch)
	}

	return ch, unsubscribe
}

// Broadcast dispatches a new WAL record to all active follower subscriber channels.
func (b *Broadcaster) Broadcast(msg *beastv1.WALRecordMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// Non-blocking drop or buffer full to protect primary from slow consumers
		}
	}
}
