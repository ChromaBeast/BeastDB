# Phase 1: Go Systems Fundamentals & Core DSA (Weeks 1–3)

Focuses on low-level memory representation, zero-copy byte semantics, and pure Go implementations of core data structures without standard library shortcuts.

---

## Week 1: Syntax & Memory Layouts

### Objectives
- Master struct memory layout, field alignment, and struct padding to minimize cache line misses.
- Understand slice header internals (`Data`, `Len`, `Cap`) and zero-allocation conversions between string and byte slices using `unsafe.Pointer`.
- Study CPU cache lines (typically 64 bytes) and prevent false sharing in concurrent data structures.

### Key Implementation Concepts
- Zero-copy string to byte conversion:
  ```go
  // UnsafeStringBytes converts a string to a byte slice without heap allocation.
  func UnsafeStringBytes(s string) []byte {
      return unsafe.Slice(unsafe.StringData(s), len(s))
  }
  ```
- Struct field reordering: order fields descending by word size (int64/pointers first, then int32, int16, int8/bool) to eliminate padding overhead.

### Milestones
1. Implement a memory inspector tool displaying struct alignment and wasted padding bytes.
2. Benchmark standard vs zero-copy conversions (`0 B/op` target).

---

## Week 2: Linear Data Structures

### Objectives
- Implement foundational linear structures with zero external dependencies and direct memory controls.

### Structures
1. **Dynamic Vector (`internal/dsa/vector.go`):**
   - Amortized $O(1)$ appends with $1.5\times$ growth factor to conserve memory over $2\times$.
   - Direct memory block indexing and shrink-to-fit capabilities.
2. **Circular Ring Buffer (`internal/dsa/ring_buffer.go`):**
   - Fixed capacity sized to powers of two ($2^n$).
   - Replace expensive `% cap` arithmetic with bitwise mask `index & (cap - 1)`.
   - Thread-safe head/tail tracking with atomic operations for producer-consumer pipelines.
3. **Binary Min/Max Heap (`internal/dsa/heap.go`):**
   - Pure array-backed priority queue.
   - Zero-allocation sift-up and sift-down routines.
   - Serves as the backbone for TTL timer queues and buffer pool replacement tracking.

---

## Week 3: Associative Structures & Profiling

### Objectives
- Build an open-addressing hash table that avoids pointer indirection and reduces garbage collector scanning overhead.

### Structures & Techniques
1. **Open-Addressing Hash Table (`internal/dsa/hash_map.go`):**
   - Single contiguous slice storing interleaved key-value slots or Robin Hood hashing.
   - Linear probing with backward shift deletion or tombstone markers.
   - Flat memory locality ensures sequential cache line loading on probe collisions.
2. **Profiling Harness:**
   - Establish benchmark suite: `go test -bench=. -benchmem ./internal/dsa/...`
   - Profiling CPU hot paths: `go test -cpuprofile=cpu.prof -bench=.`
   - Memory allocation tracking: `go test -memprofile=mem.prof -bench=.`
   - Verify zero heap allocations on lookups and reads.

---

## Success Verification
- [ ] 0 allocations per operation on hot lookup paths.
- [ ] All data structure files adhere to the strict `< 200 LoC` limit.
- [ ] Comprehensive unit test coverage (>90%) with fuzz tests for edge cases.
