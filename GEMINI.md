# Project Rules & Working Agreement

These rules govern all interactions, coding, and architecture in this project.

---

## 1. File Modularity & Readability (< 200 LoC)
* **Strict File Length Limit:** In any project, if a file is not JSON or raw data, it must strictly remain readable and **< 200 Lines of Code (LoC)**.
* **Component Decomposition:** Decompose complex systems (B+ trees, buffer pools, hash tables, page managers) into small, single-responsibility files, structs, and reusable components.

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
