# BeastDB: instructions for coding agents

Read by Codex and other agents that use `AGENTS.md`. Claude Code reads it through `CLAUDE.md`. Antigravity also reads `GEMINI.md`, which carries the same rules; keep the two in step.

## File modularity (guideline, not a hard limit)

- One responsibility per file. About **400 lines** is the point to stop and ask "does this file do more than one thing?" If yes, split along that seam (for example sync vs. lifecycle, load vs. run). If no, leave it alone.
- Never split a cohesive file, or merge small files, just to satisfy a line count.
- Exempt: generated code (`*.pb.go`), test files, JSON and raw data.
- Do not add CI or lint checks that fail on line count. A non-exempt file past about 500 lines should say why in the change description.

## Engine principles

- **WAL before page:** a data page must never reach the data file before the WAL records that describe it are durable. The buffer pool enforces this with the WAL barrier (`WAL.EnsureDurable`); keep every dirty-page write on that path.
- **Zero-allocation hot paths:** query, hashing, parsing and buffer-pinning paths avoid heap allocations; prefer byte slices and `sync.Pool`.
- **Mechanical sympathy:** align structs to avoid false sharing, prefer flat memory layouts, respect 4 KB page alignment.

## Scope and honesty

- BeastDB is a general-purpose, domain-agnostic database. Do not hard-code schema or domain rules from downstream applications; client-specific partition metadata belongs in `partitions.json` or `-partition-config`.
- Do not claim performance relative to other databases without an equivalent-workload benchmark (see `docs/benchmarking.md`). Describe planned features as planned.
- Report vulnerabilities privately; see `SECURITY.md`.

## Before you finish

Run `go build ./... && go vet ./... && go test ./... -race`. `TestEngineSystemWorkloadBenchmark` has a fixed 1000 ops/s floor and is fsync-bound, so it fails when other packages hit the disk at the same time (CI measured ~320 ops/s in parallel vs ~1700 with `-p 1` on Windows). CI runs `go test -p 1`; locally, re-run it alone before treating a failure as a regression.
