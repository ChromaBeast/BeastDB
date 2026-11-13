import { api } from "../lib/api-client";
import { parseUniversalRecord } from "../utils/data-parser";
import type { UniversalRecord } from "../types";

export type QueryMode = "get" | "scan" | "search";

/** Raw (string) form values for every mode; only the active mode's fields are read. */
export interface QueryParams {
  mode: QueryMode;
  key: string;
  start: string;
  end: string;
  limit: string;
  q: string;
  prefix: string;
  maxScan: string;
}

export interface QueryOutcome {
  records: UniversalRecord[];
  hasMore: boolean;
  nextKey?: string;
  scanned?: number;
  indexed?: boolean;
  ms: number;
}

export const EMPTY_PARAMS: QueryParams = {
  mode: "get", key: "", start: "", end: "", limit: "50", q: "", prefix: "", maxScan: "2500",
};

const U64_MAX = (1n << 64n) - 1n;

/** Parses a uint64 given as decimal or 0x-hex and returns its decimal string. */
function u64(label: string, s: string): string {
  const t = s.trim();
  if (!/^(0x[0-9a-f]+|\d+)$/i.test(t)) {
    throw new Error(`${label} must be an unsigned 64-bit integer (decimal or 0x hex).`);
  }
  const v = BigInt(t);
  if (v > U64_MAX) throw new Error(`${label} exceeds the 64-bit key space.`);
  return v.toString();
}

function posInt(label: string, s: string, max: number): number {
  const n = Number(s.trim());
  if (!Number.isInteger(n) || n < 1) throw new Error(`${label} must be a positive integer.`);
  return Math.min(n, max);
}

interface RawItem { keyText: string; value: string }
interface ListResponse {
  records: RawItem[];
  hasMore: boolean;
  nextKeyText?: string;
  nextKey?: string;
  scannedCount?: number;
  indexed?: boolean;
}

/** Short human label for a history entry. */
export function describeQuery(p: QueryParams): string {
  if (p.mode === "get") return `GET ${p.key.trim()}`;
  if (p.mode === "search") return `SEARCH "${p.q.trim()}"${p.prefix.trim() ? ` @${p.prefix.trim()}` : ""}`;
  return `SCAN ${p.start.trim() || "min"} .. ${p.end.trim() || "max"}`;
}

/** Validates the form and calls the matching existing endpoint. Throws Error with a UI-ready message. */
export async function runQuery(p: QueryParams): Promise<QueryOutcome> {
  const t0 = performance.now();
  const done = (o: Omit<QueryOutcome, "ms">): QueryOutcome => ({ ...o, ms: performance.now() - t0 });

  if (p.mode === "get") {
    if (!p.key.trim()) throw new Error("Enter a key to look up.");
    const k = u64("Key", p.key);
    const r = await api<RawItem>(`/api/key?k=${k}`);
    return done({ records: [parseUniversalRecord({ keyText: r.keyText ?? k, value: r.value })], hasMore: false });
  }

  if (p.mode === "scan") {
    const qs = new URLSearchParams({ limit: String(posInt("Limit", p.limit, 500)) });
    if (p.start.trim()) qs.set("start", u64("Start", p.start));
    if (p.end.trim()) qs.set("end", u64("End", p.end));
    const r = await api<ListResponse>(`/api/records?${qs}`);
    return done({
      records: r.records.map(parseUniversalRecord),
      hasMore: r.hasMore,
      nextKey: r.hasMore ? r.nextKeyText : undefined,
    });
  }

  const q = p.q.trim();
  if (q.length < 2) throw new Error("Search text must be at least 2 characters.");
  const qs = new URLSearchParams({ q, limit: "200", maxScan: String(posInt("Max scan", p.maxScan, 10000)) });
  if (p.prefix.trim()) {
    const n = Number(u64("Partition", p.prefix));
    if (n > 255) throw new Error("Partition prefix must be between 0 and 255.");
    qs.set("prefix", String(n));
  }
  if (p.start.trim()) qs.set("start", u64("Start", p.start));
  const r = await api<ListResponse>(`/api/search?${qs}`);
  return done({
    records: r.records.map(parseUniversalRecord),
    hasMore: r.hasMore,
    nextKey: r.hasMore ? r.nextKey : undefined,
    scanned: r.scannedCount,
    indexed: r.indexed,
  });
}
