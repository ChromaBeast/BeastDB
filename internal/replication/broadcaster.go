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

	if oldCh, exists := b.subscribers[replicaID]; exists {
		delete(b.subscribers, replicaID)
		close(oldCh)
	}

	ch := make(chan *beastv1.WALRecordMessage, bufSize)
	b.subscribers[replicaID] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if cur, ok := b.subscribers[replicaID]; ok && cur == ch {
			delete(b.subscribers, replicaID)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Broadcast dispatches a new WAL record to all active follower subscriber channels.
// If a consumer's channel is full, it is evicted and closed so the follower fails visibly and resyncs.
func (b *Broadcaster) Broadcast(msg *beastv1.WALRecordMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			delete(b.subscribers, id)
			close(ch)
		}
	}
}
