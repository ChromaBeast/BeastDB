# 🖥️ BeastDB Studio — Architecture & Universal Data Explorer

BeastDB Studio is an embedded administrative console and universal data explorer for BeastDB. It pairs low-level systems mechanics with a modern dark-mode dashboard, giving operators real-time visibility into the storage engine, buffer pool, WAL durability, and keyspace.

---

## ⚡ Zero Runtime Overhead Architecture

Unlike typical database dashboards requiring a standalone Node.js server or container, BeastDB Studio introduces **zero production runtime overhead**:

```
+-------------------------------------------------------------+
|                     Next.js 15 App (studio/)                |
|           React 19 · Tailwind CSS · Lucide Icons            |
+-------------------------------------------------------------+
                              │
                              ▼ bun run build (next export)
+-------------------------------------------------------------+
|                 Static Assets (out/ -> static/)             |
|                  HTML · Chunks · CSS · Icons                |
+-------------------------------------------------------------+
                              │
                              ▼ go:embed all:static
+-------------------------------------------------------------+
|                    Single BeastDB Binary                    |
|    Embedded HTTP Admin (Port 8088) + gRPC Engine (Port 50051)|
+-------------------------------------------------------------+
```

1. **Compilation**: Built with Next.js 15 and exported statically (`output: 'export'`) to `internal/web/static/`.
2. **Embedding**: Embedded into the Go binary via Go's `//go:embed all:static` directive.
3. **Execution**: Served directly by Go's native `http.FileServer` out of memory with zero external dependencies.

---

## 🔐 Authentication & Session Security

- **Argon2id Password Hashing**: Superuser passwords are encrypted using memory-hard Argon2id (`time=1, memory=64MB, threads=4`).
- **Cryptographic Synchronization**: On startup, `-admin-password` runtime flag automatically synchronizes the admin hash stored in the B+ Tree (`_sys:user:admin`).
- **HMAC-SHA256 Session Cookies**: Session state is signed using a 32-byte secret with `HttpOnly`, `SameSite=Strict`, and `Secure` attributes.

---

## 🧭 Universal Multi-Purpose Data Explorer

BeastDB is schema-agnostic and stores any payload type. The Studio dynamically introspects every record with **zero data loss**:

| Format | Auto-Detection Logic | UI Presentation |
|---|---|---|
| **JSON Document** | Valid `{...}` object | Deep property sheet, nested cards, image previews, date formatting |
| **Plain String / Index** | Non-JSON text | Formatted string card, character count, instant copy |
| **Delimited Token** | Pipe-separated `a\|b\|c` | Unpacked segment badges, expiration / identity chips |
| **JSON Array** | Valid `[...]` array | Indexed element list, item count pill |
| **Binary / System** | Raw bytes / System user | Byte-size counter, raw syntax viewer, hex bit-slicer |

### Dual Layout Modes
- **Data Table View**: Dense matrix displaying 64-bit key, partition badge, payload format, smart primary identifier, discovered attribute pills, and byte size.
- **Card Gallery View**: Responsive cards showcasing media cover art/posters when detected, star ratings, and prominent attribute chips.

---

## 🔬 64-Bit Key Bit-Slicer & Analyzer

BeastDB uses deterministic bit-sliced 64-bit integer keys (`prefix(8) | userHash(28) | itemHash(28)`). The Studio includes an interactive bit-slicer:
- **Prefix (High 8 bits)**: Identifies domain partition (`0x01` User, `0x03` Game, `0x04` Movie, `0x05` Token, `0x06` TV, `0x07` Book, `0x51` System).
- **Bucket (28 bits)**: User scope hash for contiguous B+ Tree clustering.
- **Item (28 bits)**: Entity ID hash for deterministic lookups.

---

## 🚀 Running BeastDB Studio

```bash
# Start BeastDB server with embedded Studio enabled on port 8088
go run ./cmd/server \
  -role leader \
  -port 50051 \
  -data-dir ./data \
  -web-addr 0.0.0.0:8088 \
  -admin-password my_secure_password
```

*(Optional: pass `-partition-config ./path/to/partitions.json` if your project defines custom partition labels).*

Open `http://localhost:8088` in your browser and sign in with username `admin`.

---

## 🔥 Firebase-Style UI & Universal Capabilities

- **3-Pane Collection Explorer**: Firebase Console style navigation: Left Partition Rail (with color-coded indicators) → Document List (with cover art/avatar badges) → Full-height Document Viewer.
- **Server-Driven Dynamic Partitions**: Any project can customize partitions via `-partition-config partitions.json` without recompiling the Studio.
- **Automated Sensitive Field Redaction**: `passwordHash`, `salt`, `token`, `secret`, and `apiKey` are stripped at parse time from property sheets.
- **Dedicated User Profile & Cascading Delete**: Selecting a User Account record (`0x01`) opens an avatar profile panel. Admins can delete a single record or perform a cascading wipe (`DELETE /api/user?userHash=...`) of all orphan records across the keyspace.
- **Server-Side Full Keyspace Search**: `GET /api/search` executes high-speed scanning across all B+ Tree leaves with optional partition filters.
- **Live Partition Distribution Donut Chart**: Dynamic SVG chart in the Overview dashboard summarizing keyspace allocation per partition in real time.
- **Direct Inline Record Editor**: Inspect, validate JSON syntax, and update records directly in place.
