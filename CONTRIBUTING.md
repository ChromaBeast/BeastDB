# Contributing to BeastDB

Thank you for your interest in contributing to BeastDB! We welcome contributions that maintain our high standard of performance, durability, and modularity.

---

## 🏛️ Engineering Standards

### 1. File Modularity (guideline, not a hard limit)
- **Principle:** One responsibility per file. Treat about **400 lines** as a prompt to ask "does this file do more than one thing?" If it does, split along that seam. If it doesn't, leave it as is. Never split, or merge, files only to hit a number.
- **Exempt:** generated code (`*.pb.go`), test files, and data files (JSON and similar).
- **Review, not CI:** there is no automated line-count gate. A non-exempt file past about 500 lines should say in the pull request why it stays one file.
- **Decomposition:** Large subsystems (B+ Tree, Buffer Pool, Hash Table, WAL) should be organized into focused, single-responsibility files and companion helpers.

### 2. Zero-Allocation Hot Paths
- Query execution, hash calculation, parsing, and buffer pinning paths must not trigger unnecessary heap allocations.
- Prefer byte slices, stack allocation, ring buffers, and `sync.Pool` over pointer-heavy structures.

### 3. Mechanical Sympathy & Durability
- Align structs to avoid false sharing across 64-byte CPU cache lines (`CacheLinePad`).
- WAL-before-page: log records must be flushed (`fsync`) before any modified in-memory database pages are written.
- All page reads/writes must respect 4KB hardware page alignment.

---

## 🔒 Security Issues

Do not report vulnerabilities in public issues or pull requests. Follow [SECURITY.md](SECURITY.md).

---

## 🧪 Verification & Testing

Before submitting a pull request, ensure all validations pass:

```bash
# 1. Run full unit and chaos test suite
go test -v -count=1 ./...

# 2. Run benchmarks to verify zero allocations on hot paths
go test -bench=. -benchmem ./...

# 3. (Optional) List large non-test, non-generated files to review. Informational only.
Get-ChildItem -Recurse -Filter "*.go" | Where-Object { $_.Name -notmatch "\.pb\.go$|_test\.go$" } | ForEach-Object {
    $lines = (Get-Content $_.FullName).Count
    if ($lines -ge 400) { Write-Host "$lines lines in $($_.FullName): does it do more than one thing?" }
}
```
