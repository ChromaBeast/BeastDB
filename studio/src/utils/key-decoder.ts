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
  0x01: { label: "User Accounts", color: "emerald", description: "User profiles and authentication credentials" },
  0x02: { label: "User by ID", color: "teal", description: "Secondary index mapping user UUIDs to emails" },
  0x03: { label: "Game Collection", color: "purple", description: "User game library entries and progress" },
  0x04: { label: "Movie Collection", color: "cyan", description: "User movie library entries and watchlist" },
  0x05: { label: "Refresh Tokens", color: "amber", description: "Hashed session and API tokens" },
  0x06: { label: "TV Collection", color: "indigo", description: "User TV show progress and episode tracking" },
  0x07: { label: "Book Collection", color: "rose", description: "User reading status and page progress" },
  0x08: { label: "Friendships", color: "blue", description: "Bidirectional social graph edges" },
  0x09: { label: "Friend Requests", color: "violet", description: "Pending and accepted friend requests" },
  0x0a: { label: "User by Username", color: "teal", description: "Secondary index mapping usernames to emails" },
  0x0b: { label: "Inbox Requests", color: "sky", description: "Incoming friend requests by receiver" },
  0x0c: { label: "Outbox Requests", color: "sky", description: "Outgoing friend requests by sender" },
};

// Mutable registry — overwritten at runtime by setPartitionRegistry.
let prefixRegistry: Record<number, PartitionMeta> = {
  ...DEFAULT_REGISTRY,
};

// setPartitionRegistry sets server-provided partition config over the defaults.
// Call this once on Studio boot after fetching GET /api/partitions.
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
    label: prefix === 0 ? "Default / Root" : `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
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
