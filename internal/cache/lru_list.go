package cache

// LRUNode represents a single doubly linked element in the LRU eviction queue.
type LRUNode[V any] struct {
	Key       string
	Value     V
	ExpiresAt int64 // Unix milliseconds; 0 means no expiration
	prev      *LRUNode[V]
	next      *LRUNode[V]
}

// LRUList is a doubly linked list maintaining items in access order.
// Head is Most Recently Used (MRU); Tail is Least Recently Used (LRU).
type LRUList[V any] struct {
	head *LRUNode[V]
	tail *LRUNode[V]
	len  int
}

// NewLRUList initializes an empty doubly linked list.
func NewLRUList[V any]() *LRUList[V] {
	return &LRUList[V]{}
}

// Len returns the count of active nodes in the list.
func (l *LRUList[V]) Len() int {
	return l.len
}

// IsHead returns true if the given node is already the Most Recently Used head.
func (l *LRUList[V]) IsHead(node *LRUNode[V]) bool {
	return l.head == node
}

// PushFront inserts a node at the head (Most Recently Used).
func (l *LRUList[V]) PushFront(node *LRUNode[V]) {
	node.prev = nil
	node.next = l.head

	if l.head != nil {
		l.head.prev = node
	}
	l.head = node

	if l.tail == nil {
		l.tail = node
	}
	l.len++
}

// MoveToFront detaches an existing node and relocates it to the head in O(1).
func (l *LRUList[V]) MoveToFront(node *LRUNode[V]) {
	if l.head == node {
		return
	}
	l.Remove(node)
	l.PushFront(node)
}

// Remove detaches a node from anywhere in the list in O(1).
func (l *LRUList[V]) Remove(node *LRUNode[V]) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}

	node.prev = nil
	node.next = nil
	l.len--
}

// RemoveTail detaches and returns the least recently used node in O(1).
func (l *LRUList[V]) RemoveTail() *LRUNode[V] {
	if l.tail == nil {
		return nil
	}
	tail := l.tail
	l.Remove(tail)
	return tail
}
