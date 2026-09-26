import type { PartitionConfig } from "../types";

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

export interface PartitionMeta {
  label: string;
  color: string;
  description?: string;
}

// Generic defaults for well-known system prefixes — project-specific labels
// should be supplied via GET /api/partitions (see setPartitionRegistry).
const DEFAULT_REGISTRY: Record<number, PartitionMeta> = {
  0x01: { label: "User Account", color: "emerald", description: "User profiles and authentication credentials" },
  0x02: { label: "Index", color: "teal", description: "Secondary index mapping user IDs" },
  0x05: { label: "Auth Token", color: "amber", description: "Short-lived session and API tokens" },
  0x08: { label: "Relationship", color: "blue", description: "Bidirectional social graph edges" },
  0x09: { label: "Pending Request", color: "violet", description: "Pending friend/follow requests" },
  0x0a: { label: "Name Index", color: "teal" },
  0x0b: { label: "Inbox", color: "sky" },
  0x0c: { label: "Outbox", color: "sky" },
  0x10: { label: "Media Catalog", color: "orange", description: "Shared media metadata (movies, games, books)" },
  0x51: { label: "System", color: "fuchsia" },
};

// Mutable registry — overwritten at runtime by setPartitionRegistry.
let prefixRegistry: Record<number, PartitionMeta> = {
  ...DEFAULT_REGISTRY,
};

// setPartitionRegistry merges server-provided partition config over the defaults.
// Call this once on Studio boot after fetching GET /api/partitions.
export function setPartitionRegistry(entries: PartitionConfig[]): void {
  const next: Record<number, PartitionMeta> = { ...DEFAULT_REGISTRY };
  for (const e of entries) {
    next[e.prefix] = { label: e.label, color: e.color, description: e.description };
  }
  prefixRegistry = next;
}

export function getPartitionMeta(prefix: number): PartitionMeta {
  return prefixRegistry[prefix] ?? {
    label: prefix === 0 ? "Default / Root" : `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
    color: "slate",
  };
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
