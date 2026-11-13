"use client";
import { useCallback, useRef, useState } from "react";
import { AlertCircle, History, Play } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { Input } from "../ui/input";
import { QueryResults } from "../QueryResults";
import { EMPTY_PARAMS, QueryMode, QueryOutcome, QueryParams, describeQuery, runQuery } from "../query-runner";
import type { ViewProps } from "./types";

const MODES: { id: QueryMode; label: string; hint: string }[] = [
  { id: "get", label: "Get key", hint: "Exact lookup in the primary B+ tree by 64-bit key." },
  { id: "scan", label: "Scan range", hint: "Ordered range scan over the primary index; leave bounds empty for the full key space." },
  { id: "search", label: "Search text", hint: "Match value text. Uses the secondary index when it can, otherwise scans up to Max scan records." },
];

const inputCls = "font-mono text-[13px]";
const kbdCls = "rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]";

export function QueryView({ onNotice }: ViewProps) {
  const [params, setParams] = useState<QueryParams>(EMPTY_PARAMS);
  const [outcome, setOutcome] = useState<QueryOutcome | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [history, setHistory] = useState<QueryParams[]>([]);
  const seq = useRef(0);

  const set = (patch: Partial<QueryParams>) => setParams((p) => ({ ...p, ...patch }));

  const run = useCallback(async (p: QueryParams) => {
    const id = ++seq.current;
    setRunning(true);
    setError(null);
    try {
      const res = await runQuery(p);
      if (id !== seq.current) return;
      setOutcome(res);
      setHistory((h) => [p, ...h.filter((x) => describeQuery(x) !== describeQuery(p))].slice(0, 10));
    } catch (e) {
      if (id !== seq.current) return;
      setOutcome(null);
      setError((e as Error).message);
      onNotice((e as Error).message, true);
    } finally {
      if (id === seq.current) setRunning(false);
    }
  }, [onNotice]);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      void run(params);
    }
  };

  const continueFrom = (nextKey: string) => {
    const next = { ...params, start: nextKey };
    setParams(next);
    void run(next);
  };

  const mode = MODES.find((m) => m.id === params.mode)!;

  return (
    <section aria-labelledby="query-title" className="space-y-6">
      <PageHeader id="query-title" title="Query" subtitle="Read records through the engine's existing lookup, range and text-search paths." />

      <Card className="p-5" onKeyDown={onKeyDown}>
        <div role="tablist" aria-label="Query mode" className="mb-4 inline-flex rounded-lg border border-border bg-muted/40 p-0.5">
          {MODES.map((m) => (
            <button
              key={m.id}
              role="tab"
              type="button"
              aria-selected={params.mode === m.id}
              onClick={() => set({ mode: m.id })}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                params.mode === m.id ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
        <p className="mb-3 text-xs text-muted-foreground">{mode.hint}</p>

        {params.mode === "get" && (
          <Input aria-label="Key" className={inputCls} placeholder="Key, e.g. 360287970189639680 or 0x0500000000000001" value={params.key} onChange={(e) => set({ key: e.target.value })} />
        )}
        {params.mode === "scan" && (
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_120px]">
            <Input aria-label="Start key" className={inputCls} placeholder="Start key (inclusive)" value={params.start} onChange={(e) => set({ start: e.target.value })} />
            <Input aria-label="End key" className={inputCls} placeholder="End key (inclusive)" value={params.end} onChange={(e) => set({ end: e.target.value })} />
            <Input aria-label="Limit" type="number" min={1} max={500} className={inputCls} placeholder="Limit" value={params.limit} onChange={(e) => set({ limit: e.target.value })} />
          </div>
        )}
        {params.mode === "search" && (
          <div className="grid gap-3 sm:grid-cols-[1fr_140px_120px]">
            <Input aria-label="Search text" className={inputCls} placeholder="Text to find in values (min 2 chars)" value={params.q} onChange={(e) => set({ q: e.target.value })} />
            <Input aria-label="Partition prefix" className={inputCls} placeholder="Prefix 0-255 (opt.)" value={params.prefix} onChange={(e) => set({ prefix: e.target.value })} />
            <Input aria-label="Max scan" type="number" min={1} max={10000} className={inputCls} placeholder="Max scan" value={params.maxScan} onChange={(e) => set({ maxScan: e.target.value })} />
          </div>
        )}

        <div className="mt-4 flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground"><kbd className={kbdCls}>Cmd/Ctrl</kbd> + <kbd className={kbdCls}>Enter</kbd> to run</span>
          <Button onClick={() => void run(params)} disabled={running}>
            <Play className="mr-1.5 h-3.5 w-3.5" aria-hidden />{running ? "Running…" : "Run"}
          </Button>
        </div>
      </Card>

      {error && (
        <div role="alert" className="flex items-start gap-2.5 rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
          <span>{error}</span>
        </div>
      )}

      {outcome && <QueryResults outcome={outcome} onContinue={continueFrom} />}
      {!outcome && !error && !running && (
        <p className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
          Run a query to see records here.
        </p>
      )}

      {history.length > 0 && (
        <Card className="p-4">
          <h2 className="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            <History className="h-3.5 w-3.5" aria-hidden />Recent queries
          </h2>
          <ul className="space-y-0.5">
            {history.map((h) => (
              <li key={describeQuery(h)}>
                <button
                  type="button"
                  onClick={() => { setParams(h); void run(h); }}
                  className="w-full truncate rounded-md px-2 py-1.5 text-left font-mono text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  {describeQuery(h)}
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </section>
  );
}
