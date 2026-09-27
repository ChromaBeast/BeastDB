import { UniversalRecord } from "../types";

export interface PartitionMeta {
  label: string;
  color: string;
  description?: string;
}

const PALETTE = [
  "emerald", "purple", "cyan", "blue", "indigo",
  "rose", "amber", "teal", "violet", "sky", "orange",
];

export function getDeterministicColor(prefix: number): string {
  return PALETTE[prefix % PALETTE.length];
}

function pluralize(word: string): string {
  if (word.endsWith("s") || word.endsWith("sh") || word.endsWith("ch")) return `${word}es`;
  if (word.endsWith("y") && !/[aeiou]y$/i.test(word)) return `${word.slice(0, -1)}ies`;
  return `${word}s`;
}

export function formatCollectionName(str: string): string {
  const cleaned = str.replace(/[_-]/g, " ").trim();
  const words = cleaned.split(/\s+/).map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase());
  const formatted = words.join(" ");
  return words.length === 1 ? pluralize(formatted) : formatted;
}

export function inferCollectionName(records: UniversalRecord[]): string | undefined {
  for (const r of records) {
    if (!r.fields) continue;
    const f = r.fields;

    // 1. Discriminator tags
    const tag = f.collection || f.type || f.kind || f.entity || f.model || f._type || f.table;
    if (typeof tag === "string" && tag.trim()) {
      return formatCollectionName(tag);
    }

    // 2. Generic *Id / *_id pattern (e.g. productId -> Products, gameId -> Games)
    for (const key of Object.keys(f)) {
      const match = key.match(/^([a-zA-Z0-9]+?)(?:Id|_id)$/i);
      if (match && match[1]) {
        const noun = match[1].toLowerCase();
        if (noun !== "id" && noun !== "item") {
          if (noun === "media") return "Media Catalog";
          if (noun === "tv" || noun === "show") return "TV Shows";
          return formatCollectionName(noun);
        }
      }
    }

    // 3. Strong semantic signatures
    if (f.email && (f.username || f.name || f.role || f.passwordHash || f.password_hash)) {
      return "Users";
    }
    if (f.sku || (f.price !== undefined && (f.category || f.inStock !== undefined))) {
      return "Products";
    }
    if (f.director || f.format === "4K HDR" || f.runtime) {
      return "Movies";
    }
    if (f.isbn || f.author || f.pages) {
      return "Books";
    }
    if (f.token || f.sessionToken || f.refreshToken || f.token_hash) {
      return "Tokens";
    }
    if (f.friendId || f.friend_id || f.relationship) {
      return "Friendships";
    }
    if (f.temperature || f.humidity || f.sensorId) {
      return "Telemetry";
    }
  }
  return undefined;
}

export function inferPartitionMeta(records: UniversalRecord[], prefix: number): PartitionMeta {
  const inferredLabel = inferCollectionName(records);
  return {
    label: inferredLabel ?? `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
    color: getDeterministicColor(prefix),
  };
}
