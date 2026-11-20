import type { CollectionConfig, PartitionConfig, UniversalRecord } from "../types";
import {
  type PartitionMeta,
  getDeterministicColor,
  inferPartitionMeta,
} from "./schema-inference";

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

const CATALOG_PREFIX = 0xff;

let prefixRegistry: Record<number, PartitionMeta> = {};
let inferredRegistry: Record<number, PartitionMeta> = {};
// Named collections from the server catalog (ADR 0002). They win over partitions.json.
let collectionRegistry: Record<number, string> = {};

export function setCollectionRegistry(entries: CollectionConfig[]): void {
  const next: Record<number, string> = {};
  for (const e of entries ?? []) next[e.prefix] = e.name;
  collectionRegistry = next;
}

export function getCollectionName(prefix: number): string | undefined {
  return collectionRegistry[prefix];
}

export function setPartitionRegistry(entries: PartitionConfig[]): void {
  const next: Record<number, PartitionMeta> = {};
  for (const e of entries ?? []) {
    next[e.prefix] = { label: e.label, color: e.color, description: e.description };
  }
  prefixRegistry = next;
}

export function setInferredPartitionRegistry(entries: Record<number, PartitionMeta>): void {
  inferredRegistry = entries;
}

export function getPartitionMeta(prefix: number): PartitionMeta {
  const name = collectionRegistry[prefix];
  if (name) {
    const base = prefixRegistry[prefix] ?? inferredRegistry[prefix];
    return { label: name, color: base?.color ?? getDeterministicColor(prefix), description: base?.description };
  }
  if (prefix === CATALOG_PREFIX) {
    // Reserved for the collection catalog record (ADR 0002); never a user collection.
    return { label: "System catalog", color: "slate", description: "Reserved: holds the collection catalog" };
  }
  return prefixRegistry[prefix] ?? inferredRegistry[prefix] ?? {
    label: `Collection ${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
    color: getDeterministicColor(prefix),
  };
}

export function getRegisteredPartitions(): Array<{ prefix: number; label: string; color: string; description?: string }> {
  const prefixes = new Set<number>([
    ...Object.keys(prefixRegistry).map(Number),
    ...Object.keys(collectionRegistry).map(Number),
  ]);
  return [...prefixes]
    .map((prefix) => ({ prefix, ...getPartitionMeta(prefix) }))
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

export function buildPartitionList(records: UniversalRecord[], partitionCounts?: Record<string, number>) {
  const map = new Map<number, { prefixLabel: string; count: number }>();

  const recordsByPrefix = new Map<number, UniversalRecord[]>();
  for (const r of records) {
    let list = recordsByPrefix.get(r.prefix);
    if (!list) {
      list = [];
      recordsByPrefix.set(r.prefix, list);
    }
    list.push(r);
  }

  const resolveMeta = (prefix: number): PartitionMeta => {
    if (collectionRegistry[prefix] || prefixRegistry[prefix] || inferredRegistry[prefix]) return getPartitionMeta(prefix);
    const recs = recordsByPrefix.get(prefix);
    return recs?.length ? inferPartitionMeta(recs, prefix) : getPartitionMeta(prefix);
  };

  for (const r of getRegisteredPartitions()) {
    const count = partitionCounts ? (partitionCounts[String(r.prefix)] ?? 0) : 0;
    map.set(r.prefix, { prefixLabel: r.label, count });
  }

  for (const [rawPrefix, count] of Object.entries(partitionCounts ?? {})) {
    const prefix = Number(rawPrefix);
    if (!map.has(prefix)) {
      const meta = resolveMeta(prefix);
      map.set(prefix, { prefixLabel: meta.label, count });
    }
  }

  for (const r of records) {
    const existing = map.get(r.prefix);
    if (existing) {
      if (!partitionCounts) existing.count += 1;
      if (existing.prefixLabel.startsWith("Collection ")) {
        const meta = resolveMeta(r.prefix);
        existing.prefixLabel = meta.label;
      }
    } else {
      const count = partitionCounts ? (partitionCounts[String(r.prefix)] ?? 1) : 1;
      const meta = resolveMeta(r.prefix);
      map.set(r.prefix, { prefixLabel: meta.label, count });
    }
  }

  const names = new Map<string, number>();
  for (const item of map.values()) names.set(item.prefixLabel, (names.get(item.prefixLabel) ?? 0) + 1);
  return [...map.entries()].sort(([a], [b]) => a - b).map(([prefix, g]) => ({
    prefix, count: g.count,
    prefixLabel: (names.get(g.prefixLabel) ?? 0) > 1
      ? `${g.prefixLabel} · ${prefix.toString(16).padStart(2, "0").toUpperCase()}`
      : g.prefixLabel,
  }));
}
