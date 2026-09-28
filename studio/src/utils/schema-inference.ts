import { UniversalRecord } from "../types";

export interface PartitionMeta {
  label: string;
  color: string;
  description?: string;
}

const PALETTE = ["emerald", "purple", "cyan", "blue", "indigo", "rose", "amber", "teal", "violet", "sky", "orange"];
const RELATION_IDS = new Set(["user", "owner", "sender", "receiver", "actor", "target", "friend", "parent", "createdby", "updatedby"]);
const GENERIC_OBJECTS = new Set(["data", "payload", "metadata", "details", "attributes", "fields"]);
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const UUID_HEX_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$|^[0-9a-f]{32,64}$/i;

export function getDeterministicColor(prefix: number): string {
  return PALETTE[prefix % PALETTE.length];
}

function pluralize(word: string): string {
  if (/^media$/i.test(word)) return word;
  if (/s$/i.test(word)) return word;
  if (/(sh|ch|x|z)$/i.test(word)) return `${word}es`;
  if (/[^aeiou]y$/i.test(word)) return `${word.slice(0, -1)}ies`;
  return `${word}s`;
}

export function formatCollectionName(str: string): string {
  const cleaned = str.replace(/([a-z])([A-Z])/g, "$1 $2").replace(/[_-]/g, " ").trim();
  const words = cleaned.split(/\s+/).filter(Boolean).map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase());
  if (words.length === 0) return "";
  return [...words.slice(0, -1), pluralize(words[words.length - 1])].join(" ");
}

function nameFromFields(f: Record<string, unknown>): { label: string; confidence: number } | undefined {
  const tag = f.collection ?? f.entity ?? f.model ?? f.table ?? f._type;
  if (typeof tag === "string" && tag.trim()) return { label: formatCollectionName(tag), confidence: 5 };

  for (const [key, value] of Object.entries(f)) {
    if (value && typeof value === "object" && !Array.isArray(value)
      && !GENERIC_OBJECTS.has(key.toLowerCase()) && !RELATION_IDS.has(key.toLowerCase())) {
      const nested = value as Record<string, unknown>;
      if (typeof nested.title === "string" || typeof nested.name === "string" || nested.id != null) {
        return { label: formatCollectionName(key), confidence: 4 };
      }
    }
  }

  const keys = Object.keys(f);
  // Generic domain-agnostic schema heuristics
  if (f.sku != null && f.price != null) return { label: "Products", confidence: 4 };
  if (f.sensorId != null && (f.temperature != null || f.humidity != null)) return { label: "Telemetry", confidence: 4 };
  if (f.email != null && (f.username != null || f.passwordHash != null || f.password_hash != null)) return { label: "Users", confidence: 4 };
  if (f.username != null && f.role != null && f.email == null) return { label: "System Users", confidence: 4 };
  if (f.token != null || f.tokenHash != null || f.token_hash != null || f.refreshToken != null || f.refresh_token != null) {
    return { label: "Tokens", confidence: 4 };
  }

  const ids = keys.map((key) => key.match(/^(.+?)(?:Id|_id)$/i)?.[1]).filter((name): name is string => Boolean(name));
  const subjects = ids.filter((name) => !RELATION_IDS.has(name.toLowerCase()) && name.toLowerCase() !== "item");
  if (subjects.length === 1) return { label: formatCollectionName(subjects[0]), confidence: 3 };
  const weakTag = f.type ?? f.kind;
  if (typeof weakTag === "string" && weakTag.trim()) return { label: formatCollectionName(weakTag), confidence: 2 };
  return undefined;
}

function nameFromRecord(record: UniversalRecord): { label: string; confidence: number } | undefined {
  if (record.fields) return nameFromFields(record.fields);
  if (record.format === "token" || (record.format === "string" && record.raw.includes("|") && record.raw.length < 250)) {
    return { label: "Tokens", confidence: 3 };
  }
  if (record.format === "string") {
    const raw = record.raw.trim();
    if (EMAIL_RE.test(raw)) return { label: "User Index", confidence: 3 };
    if (UUID_HEX_RE.test(raw)) return { label: "Index Pointers", confidence: 2 };
  }
  return undefined;
}

export function inferCollectionName(records: UniversalRecord[]): string | undefined {
  const scores = new Map<string, { score: number; count: number; confidence: number }>();
  let readable = 0;
  for (const record of records) {
    if (record.format === "empty" || !record.raw?.trim()) continue;
    readable++;
    const candidate = nameFromRecord(record);
    if (!candidate) continue;
    const current = scores.get(candidate.label) ?? { score: 0, count: 0, confidence: 0 };
    scores.set(candidate.label, {
      score: current.score + candidate.confidence,
      count: current.count + 1,
      confidence: Math.max(current.confidence, candidate.confidence),
    });
  }
  const best = [...scores].sort((a, b) => b[1].score - a[1].score)[0];
  if (!best || (best[1].confidence <= 2 && best[1].count !== readable)) return undefined;
  return best[0];
}

export function inferPartitionMeta(records: UniversalRecord[], prefix: number): PartitionMeta {
  return {
    label: inferCollectionName(records) ?? `Collection ${prefix.toString(16).padStart(2, "0").toUpperCase()}`,
    color: getDeterministicColor(prefix),
  };
}
