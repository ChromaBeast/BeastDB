# Phase 2: Networking & In-Memory Store (Weeks 4–7)

Builds the network communication layer, protocol parsing engines, and concurrent in-memory caching subsystem.

---

## Week 4: Custom TCP Socket Engine

### Objectives
- Establish an asynchronous or worker-pool connection listener over raw TCP sockets using `net.Listen`.
- Implement a binary framing protocol with header validation and CRC checks.

### Protocol Framing Design
```
+---------------+---------------+--------------------+---------------+---------------+
| Magic (1B)    | OpCode (1B)   | Payload Len (4B)   | CRC32 (4B)    | Payload (...) |
+---------------+---------------+--------------------+---------------+---------------+
```
- Magic Byte (`0xDB`): Prevents invalid connections and packet misalignment.
- OpCode: Identifies read/write/management commands (`0x01 = GET`, `0x02 = SET`, `0x03 = DEL`).
- CRC32: Verifies packet integrity over the wire before parsing.

---

## Week 5: RESP (Redis Serialization Protocol) Parser

### Objectives
- Support standard Redis client compatibility (`redis-cli`, Python/Node redis clients).
- Construct a zero-allocation RESP2/RESP3 streaming parser using `bufio.Reader` and byte slice windowing.

### Supported Types
1. **Simple Strings (`+`):** Status responses (e.g. `+OK\r\n`).
2. **Errors (`-`):** Protocol/engine errors (e.g. `-ERR unknown command\r\n`).
3. **Integers (`:`):** 64-bit integer values (e.g. `:1000\r\n`).
4. **Bulk Strings (`$`):** Length-prefixed binary safe data (e.g. `$5\r\nhello\r\n`).
5. **Arrays (`*`):** Multi-bulk command lists (e.g. `*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n`).

---

## Week 6: Concurrency Primitives & Sharded Locks

### Objectives
- Prevent lock contention bottlenecks under heavy multi-client read/write concurrency.
- Eliminate global mutex bottlenecks through lock striping.

### Implementation Patterns
1. **Partitioned / Sharded Map:**
   - Hash keys to determine shard index: `shardIdx = hash(key) & (shardCount - 1)`.
   - Each shard owns an independent `sync.RWMutex` protecting its local data bucket.
2. **Lock-Free Atomic Telemetry:**
   - Track live connections, command counters, and bytes transferred using `sync/atomic`.
   - Prevent mutex overhead on diagnostic and metrics paths.

---

## Week 7: In-Memory Caching (LRU & TTL)

### Objectives
- Combine dynamic lookup with $O(1)$ LRU eviction and proactive TTL expiration.

### Submodules
1. **LRU Doubly Linked List (`internal/cache/lru.go`):**
   - Node containing key, value, and pointers (`prev`, `next`).
   - Hash map storing pointers to nodes for $O(1)$ access and head promotion.
   - Tail eviction triggered when cache exceeds designated memory limit.
2. **TTL Expiration Strategies (`internal/cache/ttl.go`):**
   - **Passive Expiration:** Check key expiration timestamp on `GET`; purge if expired.
   - **Active Expiration:** Background ticker sampling random keys (or traversing a min-heap) to purge expired keys periodically.

---

## Success Verification
- [ ] Connect with standard `redis-cli` and execute `SET`, `GET`, `DEL`, `PING`.
- [ ] Handle 10,000 concurrent client connections without deadlocks or goroutine leaks.
- [ ] LRU evicts least recently accessed keys accurately under memory pressure.
- [ ] All code files remain strictly under 200 LoC.
