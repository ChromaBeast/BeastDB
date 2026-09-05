# Phase 4: Scaling, Mobile Sync & Deployment (Weeks 13–16)

Expands the database engine into a production-ready distributed system with gRPC streaming, mobile delta synchronization, containerization, and high-load stress testing.

---

## Week 13: gRPC API & Streaming Engine

### Objectives
- Expose typed gRPC endpoints using Protocol Buffers (`proto3`).
- Implement streaming replication and Change-Data-Capture (CDC) feeds.

### Components
1. **gRPC Interface (`internal/api/rpc.go`):**
   - Unary RPCs: `Get(KeyRequest) -> ValueResponse`, `Put(PutRequest) -> Status`.
   - Streaming RPCs: `Subscribe(StreamRequest) -> stream MutationEvent`.
2. **Security & Throttling (`internal/api/auth.go`):**
   - API Key & Bearer Token authentication via gRPC metadata interceptors.
   - Token-bucket rate limiter preventing client starvation and denial of service.

---

## Week 14: Client SDK & Mobile Synchronization

### Objectives
- Provide a resilient Go client package for application backends.
- Formulate an LSN-based delta sync protocol tailored for intermittent mobile connectivity.

### Features
1. **Go Client Package (`pkg/client`):**
   - Connection pooling with automatic dead-connection pruning.
   - Automatic retries with exponential backoff and randomized jitter.
2. **Mobile Delta Sync Protocol:**
   - Client sends `LastKnownLSN`.
   - Server streams all WAL records where `LSN > LastKnownLSN`.
   - Client applies records locally into SQLite/local store, enabling offline-first operation.

---

## Week 15: Containerization & VPS Deployment

### Objectives
- Package the database into an ultra-lean, secure container.
- Configure persistent host volume mounts and native Linux `systemd` service management.

### Deliverables
1. **Multi-Stage Distroless Dockerfile:**
   - Builder stage: `golang:1.26-alpine` compiling statically linked binary (`CGO_ENABLED=0`).
   - Final stage: `gcr.io/distroless/static-debian12` resulting in ~15MB image with no shell or package manager.
2. **Production Systemd Service Unit (`customdb.service`):**
   - Process supervisor with `Restart=on-failure` and `RestartSec=5s`.
   - File descriptor and resource hardening (`LimitNOFILE=65536`, `MemoryMax=4G`).
   - Signal handling: Intercept `SIGTERM` and `SIGINT` for graceful WAL flushing and page unpinning before exit.

---

## Week 16: Profiling, Load Testing & Hardening

### Objectives
- Stress-test system to identify hot locks, memory leaks, and CPU bottlenecks.

### Testing & Tuning Strategy
1. **Load Generation:**
   - Execute multi-client concurrent benchmarks using `wrk` and `k6`.
   - Target metrics: >50,000 QPS with p99 latency < 2ms under 80% read / 20% write workload.
2. **Runtime Diagnostics:**
   - Analyze CPU flame graphs with `go tool pprof http://localhost:6060/debug/pprof/profile`.
   - Inspect allocation hot paths with `pprof -alloc_space`.
   - Check mutex lock contention with `pprof -mutex`.
3. **GC Optimization:**
   - Tune `GOMEMLIMIT` and `GOGC` to stabilize garbage collector pause times.

---

## Success Verification
- [ ] Container image build succeeds under 25MB total footprint.
- [ ] Server cleanly flushes and terminates on `SIGTERM` without corrupting active pages.
- [ ] Sustained 50k+ QPS with p99 latency under 2ms.
- [ ] All source files modularized to `< 200 LoC`.
