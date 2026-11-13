import { describe, expect, test } from "bun:test";
import { filterEntries, mergeEntries, toLogText, type LogEntry, type LogLevel } from "./logs";

const e = (seq: number, level: LogLevel, message: string): LogEntry => ({
  seq,
  time: "2026-01-01T10:00:00Z",
  level,
  message,
});

describe("logs helpers", () => {
  test("mergeEntries dedupes by seq and trims to the newest", () => {
    const a = [e(1, "info", "a"), e(2, "info", "b")];
    const merged = mergeEntries(a, [e(2, "info", "b"), e(3, "warn", "c"), e(4, "info", "d")], 3);
    expect(merged.map((x) => x.seq)).toEqual([2, 3, 4]);
    expect(mergeEntries(a, [])).toBe(a);
  });

  test("filterEntries applies level and case-insensitive text", () => {
    const all = [e(1, "info", "Server started"), e(2, "error", "Disk FULL"), e(3, "warn", "slow")];
    const levels = new Set<LogLevel>(["error", "warn"]);
    expect(filterEntries(all, levels, "").map((x) => x.seq)).toEqual([2, 3]);
    expect(filterEntries(all, levels, "disk").map((x) => x.seq)).toEqual([2]);
  });

  test("toLogText emits one line per entry", () => {
    expect(toLogText([e(1, "warn", "hi")])).toBe("2026-01-01T10:00:00Z WARN  hi\n");
    expect(toLogText([])).toBe("");
  });
});
