import type { PartitionConfig } from "../types";
import { DEFAULT_REGISTRY, type PartitionMeta } from "./default-partitions";

export type { PartitionMeta };

export interface KeyDecoded {
  raw: string;
  hex: string;
  prefix: number;
  prefixHex: string;
  prefixLabel: string;
  color: string;
  userHash: number;
  itemHash: number;
}

// Fallback to DEFAULT_REGISTRY unless custom partitions are provided by GET /api/partitions.
let prefixRegistry: Record<number, PartitionMeta> = { ...DEFAULT_REGISTRY };

export function setPartitionRegistry(entries: PartitionConfig[]): void {
  if (!entries || entries.length === 0) {
    prefixRegistry = { ...DEFAULT_REGISTRY };
    return;
  }
  const next: Record<number, PartitionMeta> = {};
  for (const e of entries) {
    next[e.prefix] = { label: e.label, color: e.color, description: e.description };
  }
  prefixRegistry = next;
}

export function getPartitionMeta(prefix: number): PartitionMeta {
  return prefixRegistry[prefix] ?? {
    label: `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
    color: "slate",
  };
}

export function getRegisteredPartitions(): Array<{ prefix: number; label: string; color: string; description?: string }> {
  return Object.entries(prefixRegistry)
    .map(([prefix, meta]) => ({
      prefix: Number(prefix),
      label: meta.label,
      color: meta.color,
      description: meta.description,
    }))
    .sort((a, b) => a.prefix - b.prefix);
}

export function decodeKey(key: string): KeyDecoded {
  try {
    const bKey = BigInt(key);
    const prefix = Number((bKey >> 56n) & 0xffn);
    const userHash = Number((bKey >> 28n) & 0x0fffffffn);
    const itemHash = Number(bKey & 0x0fffffffn);
    const hex = "0x" + bKey.toString(16).padStart(16, "0");
    const prefixHex = "0x" + prefix.toString(16).padStart(2, "0").toUpperCase();
    const meta = getPartitionMeta(prefix);
    return {
      raw: String(key),
      hex,
      prefix,
      prefixHex,
      prefixLabel: meta.label,
      color: meta.color,
      userHash,
      itemHash,
    };
  } catch {
    return {
      raw: String(key),
      hex: "0x0",
      prefix: 0,
      prefixHex: "0x00",
      prefixLabel: "Generic Key",
      color: "slate",
      userHash: 0,
      itemHash: 0,
    };
  }
}

export function formatKeyCompact(key: string): string {
  const str = String(key);
  if (str.length > 14) return str.slice(0, 5) + "..." + str.slice(-4);
  return str;
}

export function buildPartitionList(records: Array<{ prefix: number; prefixLabel: string }>, partitionCounts?: Record<string, number>) {
  const map = new Map<number, { prefixLabel: string; count: number }>();
  for (const r of getRegisteredPartitions()) {
    const count = partitionCounts ? (partitionCounts[String(r.prefix)] ?? 0) : 0;
    map.set(r.prefix, { prefixLabel: r.label, count });
  }
  for (const [rawPrefix, count] of Object.entries(partitionCounts ?? {})) {
    const prefix = Number(rawPrefix);
    if (!map.has(prefix)) {
      map.set(prefix, { prefixLabel: getPartitionMeta(prefix).label, count });
    }
  }
  for (const r of records) {
    const existing = map.get(r.prefix);
    if (existing) {
      if (!partitionCounts) existing.count += 1;
    } else {
      const count = partitionCounts ? (partitionCounts[String(r.prefix)] ?? 1) : 1;
      map.set(r.prefix, { prefixLabel: r.prefixLabel, count });
    }
  }
  return [...map.entries()].sort(([a], [b]) => a - b).map(([prefix, g]) => ({ prefix, count: g.count, prefixLabel: g.prefixLabel }));
}
