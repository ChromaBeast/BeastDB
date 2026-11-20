export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  seq: number;
  time: string;
  level: LogLevel;
  /** Derived server-side from the message text; absent from older servers. */
  source?: string;
  message: string;
}

export const LOG_SOURCES = ["engine", "grpc", "auth", "replication", "studio", "server"] as const;

/** Time-range choices for the Logs page; 0 means everything kept in memory. */
export const LOG_RANGES: { label: string; ms: number }[] = [
  { label: "All retained", ms: 0 },
  { label: "Last 5 minutes", ms: 5 * 60_000 },
  { label: "Last hour", ms: 60 * 60_000 },
];

/** Most entries the Logs page keeps in memory (matches the server ring). */
export const MAX_CLIENT_ENTRIES = 2000;

/** Appends new entries (deduped by seq) and trims to the newest `max`. */
export function mergeEntries(prev: LogEntry[], incoming: LogEntry[], max = MAX_CLIENT_ENTRIES): LogEntry[] {
  if (incoming.length === 0) return prev;
  const last = prev.length ? prev[prev.length - 1].seq : 0;
  const fresh = incoming.filter((e) => e.seq > last);
  if (fresh.length === 0) return prev;
  const next = prev.concat(fresh);
  return next.length > max ? next.slice(next.length - max) : next;
}

export interface LogFilter {
  levels: ReadonlySet<LogLevel>;
  text?: string;
  /** Exact source, or "" / undefined for all. */
  source?: string;
  /** Only entries newer than now - rangeMs; 0 / undefined keeps all. */
  rangeMs?: number;
  now?: number;
}

export function filterEntries(entries: LogEntry[], f: LogFilter): LogEntry[] {
  const q = (f.text ?? "").trim().toLowerCase();
  const cutoff = f.rangeMs ? (f.now ?? Date.now()) - f.rangeMs : 0;
  return entries.filter((e) => {
    if (!f.levels.has(e.level)) return false;
    if (f.source && e.source !== f.source) return false;
    if (q !== "" && !e.message.toLowerCase().includes(q)) return false;
    if (cutoff) {
      const t = new Date(e.time).getTime();
      if (!isNaN(t) && t < cutoff) return false;
    }
    return true;
  });
}

export function formatLogTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const p = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/** One plain-text line per entry, for the download. */
export function toLogText(entries: LogEntry[]): string {
  return entries.map((e) => `${e.time} ${e.level.toUpperCase().padEnd(5)} ${e.source ? e.source.padEnd(11) + " " : ""}${e.message}`).join("\n") + (entries.length ? "\n" : "");
}

/** RFC 4180 CSV (time, level, source, message) for the Export CSV button. Fields are quoted and quotes doubled. */
export function toLogCsv(entries: LogEntry[]): string {
  const q = (v: string) => `"${v.replace(/"/g, '""')}"`;
  const rows = entries.map((e) => [q(e.time), q(e.level), q(e.source ?? ""), q(e.message)].join(","));
  return ["time,level,source,message", ...rows].join("\r\n") + "\r\n";
}
