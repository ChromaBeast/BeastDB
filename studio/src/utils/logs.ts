export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  seq: number;
  time: string;
  level: LogLevel;
  message: string;
}

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

export function filterEntries(entries: LogEntry[], levels: ReadonlySet<LogLevel>, text: string): LogEntry[] {
  const q = text.trim().toLowerCase();
  return entries.filter((e) => levels.has(e.level) && (q === "" || e.message.toLowerCase().includes(q)));
}

export function formatLogTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const p = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/** One plain-text line per entry, for the download. */
export function toLogText(entries: LogEntry[]): string {
  return entries.map((e) => `${e.time} ${e.level.toUpperCase().padEnd(5)} ${e.message}`).join("\n") + (entries.length ? "\n" : "");
}

/** RFC 4180 CSV (time, level, message) for the Export CSV button. Fields are quoted and quotes doubled. */
export function toLogCsv(entries: LogEntry[]): string {
  const q = (v: string) => `"${v.replace(/"/g, '""')}"`;
  const rows = entries.map((e) => [q(e.time), q(e.level), q(e.message)].join(","));
  return ["time,level,message", ...rows].join("\r\n") + "\r\n";
}
