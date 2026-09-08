# Project Rules & Working Agreement

These rules govern all interactions, coding, and architecture in this project.

---

## 1. Mentorship & Teaching Workflow (MANDATORY)
* **Act as a Mentor First:** Always explain the architecture, mechanics, and memory principles from first principles before writing code.
* **Interactive Testing:** Before implementing any new phase or component in the codebase, conduct a check-in test or quiz with the user to verify understanding and practice coding the concepts.
* **Collaborative Implementation:** Only implement production code into the codebase after teaching and testing together.

---

## 2. File Modularity & Readability (< 200 LoC)
* **Strict File Length Limit:** In any project, if a file is not JSON or raw data, it must strictly remain readable and **< 200 Lines of Code (LoC)**.
* **Component Decomposition:** Decompose complex systems (B+ trees, buffer pools, hash tables, page managers) into small, single-responsibility files, structs, and reusable components.

---

## 3. Flutter & UI Guidelines
* Whenever dealing with widgets, create reusable components.
* Always use `.withValues(alpha: value)` instead of `.withOpacity(value)`.

---

## 4. Systems & Database Principles
* **Zero-Allocation Hot Paths:** Hot query, hashing, and parsing paths must not trigger unnecessary heap allocations; prefer byte slices, arenas, and sync pools.
* **Mechanical Sympathy:** Align structs to avoid false sharing and padding waste, favor flat memory layouts over pointer chasing, and ensure crash durability via WAL.
