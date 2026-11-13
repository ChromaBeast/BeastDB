"use client";
import { Card } from "./ui/card";
import { Badge } from "./ui/badge";
import { formatBytes } from "../utils/format";
import type { StorageMetrics } from "../types";

// Layout constants from internal/index (leaf_node.go, internal_node.go, node_header.go).
const PAGE_SIZE = 4096;
const MAX_LEAF_ENTRIES = 200;
const MAX_INTERNAL_KEYS = 200;

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-border/60 py-2 last:border-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-right font-mono text-xs text-foreground">{value}</dd>
    </div>
  );
}

const n = (v: number | undefined) => (v === undefined ? "—" : v.toLocaleString());

export function PrimaryIndexCard({ storage }: { storage?: StorageMetrics }) {
  const pages = storage ? Math.floor(storage.diskSizeBytes / PAGE_SIZE) : undefined;
  return (
    <Card className="p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-foreground">Primary index</h2>
        <Badge variant="mono">B+ tree</Badge>
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        Disk-backed B+ tree over 64-bit keys. Leaves are chained by next-page pointers, which range scans follow.
      </p>
      <dl>
        <Fact label="Key width" value="64-bit" />
        <Fact label="Page size" value="4 KiB" />
        <Fact label="Leaf entry" value="18 B (8 B key + 10 B RID)" />
        <Fact label="Max entries / leaf" value={String(MAX_LEAF_ENTRIES)} />
        <Fact label="Max keys / internal node" value={`${MAX_INTERNAL_KEYS} (fanout ${MAX_INTERNAL_KEYS + 1})`} />
        <Fact label="Pages in data file (≈ size / 4 KiB)" value={n(pages)} />
        <Fact label="Data file size" value={storage ? formatBytes(storage.diskSizeBytes) : "—"} />
      </dl>
    </Card>
  );
}

export function BufferPoolCard({ storage }: { storage?: StorageMetrics }) {
  const usage = storage && storage.poolSize > 0 ? Math.min(100, (storage.cachedPages / storage.poolSize) * 100) : 0;
  return (
    <Card className="p-5">
      <h2 className="mb-3 text-sm font-semibold text-foreground">Buffer pool</h2>
      <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden>
        <div className="h-full rounded-full bg-primary" style={{ width: `${usage}%` }} />
      </div>
      <dl>
        <Fact label="Pool size (pages)" value={n(storage?.poolSize)} />
        <Fact label="Cached pages" value={n(storage?.cachedPages)} />
        <Fact label="Dirty pages" value={n(storage?.dirtyPages)} />
        <Fact label="Pinned pages" value={n(storage?.pinnedPages)} />
        <Fact label="Active data page" value={n(storage?.activeDataPage)} />
      </dl>
    </Card>
  );
}

export function SecondaryIndexCard() {
  return (
    <Card className="p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-foreground">Secondary text index</h2>
        <Badge variant="mono">inverted · in-memory</Badge>
      </div>
      <p className="mb-3 text-xs text-muted-foreground">
        Maps each token to a sorted list of primary keys. It is rebuilt from stored records when the engine starts and
        kept current on every write and delete.
      </p>
      <dl>
        <Fact label="Tokenizer" value="letters + digits, lowercased" />
        <Fact label="Min token length" value="2" />
        <Fact label="Token identity" value="64-bit FNV-1a hash" />
        <Fact label="Multi-word query" value="intersection (AND)" />
      </dl>
      <p className="mt-3 text-xs text-muted-foreground">
        Index size and posting counts are not exposed by the API, so they are not shown.
      </p>
    </Card>
  );
}
