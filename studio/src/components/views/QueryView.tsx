"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, History, Play } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { Input } from "../ui/input";
import { QueryResults } from "../QueryResults";
import { CopyCommand } from "../CopyCommand";
import { api } from "../../lib/api-client";
import type { IndexEntry } from "../../types";
import { FilterOp, OP_LABEL, beastctlCommand, buildFilterQuery, httpRequest, pythonSnippet } from "../../utils/query-builder";
import { EMPTY_PARAMS, QueryMode, QueryOutcome, QueryParams, describeQuery, runQuery } from "../query-runner";
import type { ViewProps } from "./types";

const MODES: { id: QueryMode; label: string; hint: string }[] = [
  { id: "get", label: "Get key", hint: "Exact lookup in the primary B+ tree by 64-bit key." },
  { id: "scan", label: "Scan range", hint: "Ordered range scan over the primary index; leave bounds empty for the full key space." },
  { id: "filter", label: "Filter by index", hint: "Look records up through a secondary index (ADR 0006). Equality or an inclusive range on one indexed field; results come back in key order." },
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
  const [indexes, setIndexes] = useState<IndexEntry[]>([]);
  const [indexesError, setIndexesError] = useState<string | null>(null);
  const seq = useRef(0);

  useEffect(() => {
    let live = true;
    api<IndexEntry[]>("/api/indexes")
      .then((list) => { if (live) setIndexes(list ?? []); })
      .catch((e) => { if (live) setIndexesError((e as Error).message); });
    return () => { live = false; };
  }, []);

  const chosen = indexes.find((i) => i.name === params.indexName);
  // Commands come from the same validation the Run button uses, so they never describe a query the server would reject.
  const commands = useMemo(() => {
    if (params.mode !== "filter" || !chosen) return null;
    try {
      const q = buildFilterQuery({
        index: chosen, op: params.op, value: params.value, value2: params.value2,
        limit: Math.max(1, Math.floor(Number(params.limit)) || 50),
        start: /^\d+$/.test(params.start.trim()) ? params.start.trim() : undefined,
      });
      return { beastctl: beastctlCommand(q), python: pythonSnippet(q, chosen.type), http: httpRequest(q) };
    } catch {
      return null;
    }
  }, [params, chosen]);

  const set = (patch: Partial<QueryParams>) => setParams((p) => ({ ...p, ...patch }));

  const run = useCallback(async (p: QueryParams) => {
    const id = ++seq.current;
    setRunning(true);
    setError(null);
    try {
      const res = await runQuery(p, indexes);
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
  }, [onNotice, indexes]);

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
        {params.mode === "filter" && (
          indexes.length === 0 ? (
            <p className="rounded-md border border-dashed border-border px-3 py-4 text-sm text-muted-foreground">
              {indexesError ?? "No secondary indexes yet. Create one on the Indexes page to filter by a JSON field."}
            </p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              <select
                aria-label="Index"
                className="h-9 rounded-md border border-input bg-background px-2 font-mono text-[13px]"
                value={params.indexName}
                onChange={(e) => set({ indexName: e.target.value, value: "", value2: "" })}
              >
                <option value="">Choose an index…</option>
                {indexes.map((i) => (
                  <option key={i.name} value={i.name}>{i.name} — {i.collection || `0x${i.prefix.toString(16)}`}.{i.field} ({i.type}){i.state === "ready" ? "" : " · building"}</option>
                ))}
              </select>
              <select
                aria-label="Operator"
                className="h-9 rounded-md border border-input bg-background px-2 text-[13px]"
                value={params.op}
                onChange={(e) => set({ op: e.target.value as FilterOp })}
              >
                {(Object.keys(OP_LABEL) as FilterOp[]).map((o) => <option key={o} value={o}>{OP_LABEL[o]}</option>)}
              </select>
              <Input aria-label={params.op === "between" ? "Lower bound" : "Value"} className={inputCls} placeholder={params.op === "between" ? "Lower bound" : "Value"} value={params.value} onChange={(e) => set({ value: e.target.value })} />
              {params.op === "between" && (
                <Input aria-label="Upper bound" className={`${inputCls}`} placeholder="Upper bound" value={params.value2} onChange={(e) => set({ value2: e.target.value })} />
              )}
              <Input aria-label="Limit" type="number" min={1} max={500} className={inputCls} placeholder="Limit" value={params.limit} onChange={(e) => set({ limit: e.target.value })} />
            </div>
          )
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

      {commands && (
        <Card className="space-y-3 p-5">
          <h2 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Same query elsewhere</h2>
          <CopyCommand label="beastctl" command={commands.beastctl} onNotice={onNotice} />
          <CopyCommand label="Python (gRPC stubs)" command={commands.python} onNotice={onNotice} />
          <CopyCommand label="Studio HTTP API" command={commands.http} onNotice={onNotice} />
          <p className="text-xs text-muted-foreground">beastctl reads <code className="font-mono">BEASTDB_API_TOKEN</code> from the environment. The address assumes the default gRPC port.</p>
        </Card>
      )}

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
