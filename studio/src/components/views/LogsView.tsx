"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, Download, Pause, Play, ScrollText } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { Input } from "../ui/input";
import { cn } from "@/lib/utils";
import {
  filterEntries,
  LOG_RANGES,
  LOG_SOURCES,
  formatLogTime,
  mergeEntries,
  toLogCsv,
  toLogText,
  type LogEntry,
  type LogLevel,
} from "../../utils/logs";
import type { ViewProps } from "./types";

const POLL_MS = 2000;
const LEVELS: LogLevel[] = ["error", "warn", "info", "debug"];
const LEVEL_STYLE: Record<LogLevel, string> = {
  error: "text-destructive",
  warn: "text-warning",
  info: "text-muted-foreground",
  debug: "text-muted-foreground/60",
};

const SELECT = "h-8 rounded-md border border-border bg-card px-2 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

type Status = "loading" | "ok" | "forbidden" | "error";

export function LogsView({ user }: ViewProps) {
  const isAdmin = user?.role === "admin";
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [paused, setPaused] = useState(false);
  const [levels, setLevels] = useState<Set<LogLevel>>(() => new Set(LEVELS));
  const [text, setText] = useState("");
  const [source, setSource] = useState("");
  const [rangeMs, setRangeMs] = useState(0);
  const [atBottom, setAtBottom] = useState(true);
  const latest = useRef(0);
  const scroller = useRef<HTMLDivElement>(null);

  const poll = useCallback(async () => {
    try {
      const res = await fetch(`/api/logs?since=${latest.current}&limit=1000`, { cache: "no-store" });
      if (res.status === 403 || res.status === 401) return setStatus("forbidden");
      if (!res.ok) return setStatus("error");
      const body = (await res.json()) as { entries: LogEntry[]; latest: number };
      // The server restarted (seq went backwards): start over.
      if (body.latest < latest.current) {
        latest.current = 0;
        setEntries([]);
      } else {
        latest.current = Math.max(latest.current, body.latest);
        setEntries((prev) => mergeEntries(prev, body.entries));
      }
      setStatus("ok");
    } catch {
      setStatus("error");
    }
  }, []);

  useEffect(() => {
    if (!isAdmin || paused || status === "forbidden") return;
    void poll();
    const id = setInterval(() => void poll(), POLL_MS);
    return () => clearInterval(id);
  }, [isAdmin, paused, status === "forbidden", poll]); // eslint-disable-line react-hooks/exhaustive-deps

  const visible = useMemo(
    () => filterEntries(entries, { levels, text, source, rangeMs }),
    // `entries` changes every poll, which also re-evaluates the time range.
    [entries, levels, text, source, rangeMs],
  );
  const counts = useMemo(() => {
    const c: Record<LogLevel, number> = { error: 0, warn: 0, info: 0, debug: 0 };
    for (const e of entries) c[e.level] += 1;
    return c;
  }, [entries]);

  useEffect(() => {
    const el = scroller.current;
    if (el && atBottom) el.scrollTop = el.scrollHeight;
  }, [visible, atBottom]);

  const onScroll = () => {
    const el = scroller.current;
    if (el) setAtBottom(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
  };
  const jump = () => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
    setAtBottom(true);
  };
  const toggleLevel = (l: LogLevel) =>
    setLevels((prev) => {
      const next = new Set(prev);
      if (next.has(l)) next.delete(l);
      else next.add(l);
      return next;
    });
  const download = (csv: boolean) => {
    const text = csv ? toLogCsv(visible) : toLogText(visible);
    const url = URL.createObjectURL(new Blob([text], { type: csv ? "text/csv" : "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `beastdb-logs-${new Date().toISOString().replace(/[:.]/g, "-")}.${csv ? "csv" : "txt"}`;
    a.click();
    URL.revokeObjectURL(url);
  };

  if (!isAdmin || status === "forbidden") {
    return (
      <section aria-labelledby="logs-title" className="space-y-6">
        <PageHeader id="logs-title" title="Logs" subtitle="Server log tail." />
        <Card className="p-10 text-center">
          <ScrollText className="mx-auto mb-3 h-6 w-6 text-muted-foreground" aria-hidden />
          <p className="font-medium">Logs are limited to administrators</p>
          <p className="mt-1 text-sm text-muted-foreground">Sign in as an admin to read the server log.</p>
        </Card>
      </section>
    );
  }

  return (
    <section aria-labelledby="logs-title" className="space-y-4">
      <PageHeader
        id="logs-title"
        title="Logs"
        subtitle={`Recent server log, kept in memory only (last ${entries.length} of at most 2,000 lines; cleared on restart).`}
        actions={
          <>
            <Badge variant={paused ? "mono" : "lime"}>{paused ? "PAUSED" : "LIVE"}</Badge>
            <Button variant="outline" size="sm" onClick={() => setPaused((p) => !p)}>
              {paused ? <Play className="h-3.5 w-3.5" aria-hidden /> : <Pause className="h-3.5 w-3.5" aria-hidden />}
              {paused ? "Resume" : "Pause"}
            </Button>
            <Button variant="outline" size="sm" onClick={() => download(false)} disabled={visible.length === 0}>
              <Download className="h-3.5 w-3.5" aria-hidden />
              Download
            </Button>
            <Button variant="outline" size="sm" onClick={() => download(true)} disabled={visible.length === 0}>
              Export CSV
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        {LEVELS.map((l) => {
          const on = levels.has(l);
          return (
            <button
              key={l}
              type="button"
              aria-pressed={on}
              onClick={() => toggleLevel(l)}
              className={cn(
                "rounded-md border px-2.5 py-1 font-mono text-xs uppercase transition-colors",
                on ? "border-border bg-muted text-foreground" : "border-border/50 text-muted-foreground/60 hover:text-foreground",
              )}
            >
              {l} <span className="text-muted-foreground">{counts[l]}</span>
            </button>
          );
        })}
        <select value={source} onChange={(e) => setSource(e.target.value)} aria-label="Filter by source" className={SELECT}>
          <option value="">All sources</option>
          {LOG_SOURCES.map((x) => <option key={x} value={x}>{x}</option>)}
        </select>
        <select value={rangeMs} onChange={(e) => setRangeMs(Number(e.target.value))} aria-label="Filter by time range" className={SELECT}>
          {LOG_RANGES.map((r) => <option key={r.ms} value={r.ms}>{r.label}</option>)}
        </select>
        <Input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Filter messages"
          aria-label="Filter log messages"
          className="h-8 w-full max-w-xs flex-1"
        />
      </div>

      <Card className="relative overflow-hidden">
        <div
          ref={scroller}
          onScroll={onScroll}
          role="log"
          aria-live="off"
          className="h-[calc(100vh-18rem)] min-h-[20rem] overflow-auto p-2 font-mono text-xs leading-5"
        >
          {status === "error" && (
            <p className="p-3 text-destructive">Cannot read logs right now. Retrying every 2 seconds.</p>
          )}
          {status === "loading" && <p className="p-3 text-muted-foreground">Loading logs…</p>}
          {status === "ok" && visible.length === 0 && (
            <p className="p-3 text-muted-foreground">
              {entries.length === 0 ? "No log lines yet." : "No lines match the current filters."}
            </p>
          )}
          {visible.length > 0 && (
            <div aria-hidden className="sticky top-0 z-10 flex gap-3 border-b border-border bg-card px-2 py-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              <span className="w-[4.5rem] shrink-0">Time</span>
              <span className="w-14 shrink-0">Level</span>
              <span className="w-24 shrink-0">Source</span>
              <span>Event</span>
            </div>
          )}
          {visible.map((e) => (
            <div key={e.seq} className="flex gap-3 rounded px-2 py-0.5 hover:bg-muted/50">
              <span className="w-[4.5rem] shrink-0 text-muted-foreground/70">{formatLogTime(e.time)}</span>
              <span className={cn("w-14 shrink-0 uppercase", LEVEL_STYLE[e.level])}>{e.level}</span>
              <span className="w-24 shrink-0 text-muted-foreground">{e.source ?? ""}</span>
              <span className="min-w-0 whitespace-pre-wrap break-words text-foreground">{e.message}</span>
            </div>
          ))}
        </div>
        {!atBottom && visible.length > 0 && (
          <Button size="sm" variant="secondary" onClick={jump} className="absolute bottom-3 right-4 shadow-md">
            <ArrowDown className="h-3.5 w-3.5" aria-hidden />
            Jump to latest
          </Button>
        )}
      </Card>
    </section>
  );
}
