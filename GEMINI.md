# Project Rules & Working Agreement

These rules govern all interactions, coding, and architecture in this project.

---

## 1. File Modularity & Readability (guideline, not a hard limit)
* **One responsibility per file.** Organize files by what they do. File length is a smell, not a rule: roughly **400 lines** is the point to stop and ask "does this file do more than one thing?" If yes, split it along that seam (for example sync vs. lifecycle, load vs. run). If no, leave it alone.
* **Never split just to hit a number.** Do not cut a cohesive file into arbitrary pieces, move code without a clear responsibility boundary, or merge small files back together to satisfy a count.
* **Exemptions:** generated code (`*.pb.go`), test files, and JSON/raw data have no size expectation.
* **No automated gate.** Do not add CI or lint checks that fail on line count. Past about 500 lines, mention the reason in the change description or review.
* **Component Decomposition:** Decompose complex systems (B+ trees, buffer pools, hash tables, page managers) into focused structs and reusable components.

---

## 2. Flutter & UI Guidelines
* Whenever dealing with widgets, create reusable components.
* Always use `.withValues(alpha: value)` instead of `.withOpacity(value)`.

---

## 3. Systems & Database Principles
* **Zero-Allocation Hot Paths:** Hot query, hashing, and parsing paths must not trigger unnecessary heap allocations; prefer byte slices, arenas, and sync pools.
* **Mechanical Sympathy:** Align structs to avoid false sharing and padding waste, favor flat memory layouts over pointer chasing, and ensure crash durability via WAL.

---

## 4. Architectural Generality & Application Agnosticism
* **Domain-Agnostic Core:** BeastDB (engine, gRPC API, CLI, and embedded Studio) is a general-purpose, domain-agnostic database. Never hardcode application-specific schema rules, domain models, or entity heuristics belonging to downstream client projects (such as Unfinished or external business applications).
* **Client-Driven Partition Configuration:** Any application-specific partition metadata, collection names, colors, and descriptors belong strictly on the client application side via `partitions.json` sidecar files or `-partition-config` runtime flags.
* **Multi-Repo Synchronization & Pushes:** When developing features or schemas that bridge BeastDB and downstream client applications (e.g. Unfinished), test and push changes in both repositories promptly to keep client configurations and database capabilities in sync.
