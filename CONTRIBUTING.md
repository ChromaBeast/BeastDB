# Contributing to BeastDB

Thank you for your interest in contributing to BeastDB! We welcome contributions that maintain our high standard of performance, durability, and modularity.

---

## 🏛️ Engineering Standards

### 1. Strict File Modularity (< 200 LoC)
- **Constraint:** Every source file (excluding auto-generated `.pb.go` code) must strictly remain readable and **under 200 Lines of Code (LoC)**.
- **Decomposition:** Large subsystems (B+ Tree, Buffer Pool, Hash Table, WAL) must be decomposed into focused, single-responsibility files and companion helpers.

### 2. Zero-Allocation Hot Paths
- Query execution, hash calculation, parsing, and buffer pinning paths must not trigger unnecessary heap allocations.
- Prefer byte slices, stack allocation, ring buffers, and `sync.Pool` over pointer-heavy structures.

### 3. Mechanical Sympathy & Durability
- Align structs to avoid false sharing across 64-byte CPU cache lines (`CacheLinePad`).
- WAL-before-page: log records must be flushed (`fsync`) before any modified in-memory database pages are written.
- All page reads/writes must respect 4KB hardware page alignment.

---

## 🧪 Verification & Testing

Before submitting a pull request, ensure all validations pass:

```bash
# 1. Run full unit and chaos test suite
go test -v -count=1 ./...

# 2. Run benchmarks to verify zero allocations on hot paths
go test -bench=. -benchmem ./...

# 3. Enforce the < 200 LoC modularity limit
Get-ChildItem -Recurse -Filter "*.go" | Where-Object { $_.FullName -notmatch "\.pb\.go" } | ForEach-Object {
    $lines = (Get-Content $_.FullName).Count
    if ($lines -ge 200) { Write-Error "$lines lines in $($_.Name)" }
}
```
