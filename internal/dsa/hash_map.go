package dsa

// SlotState represents the lifecycle state of a slot in the open-addressing table.
type SlotState uint8

const (
	// SlotEmpty indicates the slot has never held a value. Search terminates here.
	SlotEmpty SlotState = iota
	// SlotOccupied indicates the slot currently holds an active key-value pair.
	SlotOccupied
	// SlotTombstone indicates a previously occupied slot was deleted. Search must continue.
	SlotTombstone
)

// HashSlot represents a single entry in the flat contiguous table array.
type HashSlot[V any] struct {
	Key   string
	Value V
	State SlotState
}

// HashTable is a cache-friendly, open-addressing hash table using linear probing and tombstones.
type HashTable[V any] struct {
	slots      []HashSlot[V]
	mask       int
	count      int
	tombstones int
}

// NewHashTable creates a hash table with capacity rounded up to the nearest power of two.
func NewHashTable[V any](minCap int) *HashTable[V] {
	if minCap < 8 {
		minCap = 8
	}
	capacity := nextPowerOfTwo(minCap)
	return &HashTable[V]{
		slots: make([]HashSlot[V], capacity),
		mask:  capacity - 1,
	}
}

// Len returns the number of active key-value pairs.
func (h *HashTable[V]) Len() int {
	return h.count
}

// Cap returns the total allocated slot capacity.
func (h *HashTable[V]) Cap() int {
	return len(h.slots)
}

// Set inserts or updates a key-value pair.
func (h *HashTable[V]) Set(key string, val V) {
	if (h.count+h.tombstones+1)*10 >= len(h.slots)*7 { // 70% load factor
		h.grow()
	}

	hash := HashString(key)
	idx := int(hash) & h.mask
	firstTombstone := -1

	for {
		slot := &h.slots[idx]
		if slot.State == SlotEmpty {
			targetIdx := idx
			if firstTombstone != -1 {
				targetIdx = firstTombstone
				h.tombstones--
			}
			h.slots[targetIdx] = HashSlot[V]{Key: key, Value: val, State: SlotOccupied}
			h.count++
			return
		}

		if slot.State == SlotTombstone {
			if firstTombstone == -1 {
				firstTombstone = idx
			}
		} else if slot.Key == key {
			slot.Value = val
			return
		}

		idx = (idx + 1) & h.mask
	}
}

// Get looks up a key and returns the value and whether it was found.
func (h *HashTable[V]) Get(key string) (V, bool) {
	hash := HashString(key)
	idx := int(hash) & h.mask

	for {
		slot := &h.slots[idx]
		if slot.State == SlotEmpty {
			var zero V
			return zero, false
		}
		if slot.State == SlotOccupied && slot.Key == key {
			return slot.Value, true
		}
		idx = (idx + 1) & h.mask
	}
}

// Delete removes a key using tombstone marking, preserving probe chains.
func (h *HashTable[V]) Delete(key string) bool {
	hash := HashString(key)
	idx := int(hash) & h.mask

	for {
		slot := &h.slots[idx]
		if slot.State == SlotEmpty {
			return false
		}
		if slot.State == SlotOccupied && slot.Key == key {
			var zero V
			slot.Value = zero
			slot.Key = ""
			slot.State = SlotTombstone
			h.count--
			h.tombstones++
			return true
		}
		idx = (idx + 1) & h.mask
	}
}

func (h *HashTable[V]) grow() {
	oldSlots := h.slots
	newCap := len(oldSlots) << 1
	h.slots = make([]HashSlot[V], newCap)
	h.mask = newCap - 1
	h.count = 0
	h.tombstones = 0

	for i := range oldSlots {
		if oldSlots[i].State == SlotOccupied {
			h.Set(oldSlots[i].Key, oldSlots[i].Value)
		}
	}
}
