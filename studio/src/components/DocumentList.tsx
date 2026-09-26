"use client";
import { ArrowLeft, ArrowRight, FileText, Plus, Search } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { getPartitionMeta } from "../utils/key-decoder";

const COLOR_BG: Record<string, string> = {
  emerald: "bg-emerald-500", teal: "bg-teal-500", purple: "bg-purple-500",
  cyan: "bg-cyan-500", amber: "bg-amber-500", indigo: "bg-indigo-500",
  rose: "bg-rose-500", blue: "bg-blue-500", violet: "bg-violet-500",
  sky: "bg-sky-500", fuchsia: "bg-fuchsia-500", orange: "bg-orange-500",
  slate: "bg-slate-400",
};

interface Props {
  records: UniversalRecord[];
  selected: UniversalRecord | null;
  loading: boolean; loadingMore: boolean; hasMore: boolean;
  search: string; format: string; formats: string[];
  looking: boolean;
  title: string; shownCount: number;
  onSearchChange: (v: string) => void;
  onFormatChange: (v: string) => void;
  onExactLookup: () => void;
  onLoadMore: () => void;
  onSelect: (r: UniversalRecord) => void;
  onBack: () => void;
  canWrite: boolean;
  onNew: () => void;
}

export function DocumentList(p: Props) {
  return (
    <div aria-label="Documents" className="flex min-h-0 flex-col border-r border-border">
      <div className="border-b border-border px-4 py-4">
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" className="lg:hidden" aria-label="Back to partitions" onClick={p.onBack}>
            <ArrowLeft size={17} />
          </Button>
          <div className="min-w-0 flex-1">
            <h2 className="truncate font-semibold text-white" title={p.title}>{p.title}</h2>
            <p className="text-xs font-mono text-zinc-400">{p.shownCount} shown · {p.records.length} loaded</p>
          </div>
          {p.canWrite && (
            <Button size="sm" onClick={p.onNew} aria-label="New record" className="gap-1 text-xs">
              <Plus size={14} /> New
            </Button>
          )}
        </div>
        <div className="relative mt-3">
          <Search size={15} className="absolute left-3 top-2.5 text-zinc-500" />
          <Input aria-label="Search loaded records" value={p.search} onChange={(e) => p.onSearchChange(e.target.value)} placeholder="Search loaded records…" className="pl-9 text-xs" />
        </div>
        <div className="mt-2 flex gap-2">
          <select aria-label="Filter by format" value={p.format} onChange={(e) => p.onFormatChange(e.target.value)} className="h-9 min-w-0 flex-1 rounded-md border border-zinc-800 bg-zinc-900/90 px-2 text-xs text-zinc-300 focus:outline-none focus:ring-1 focus:ring-beast-lime">
            <option value="all">All formats</option>
            {p.formats.map((f) => <option key={f} value={f}>{f.replace("_", " ")}</option>)}
          </select>
          <Button variant="outline" size="sm" disabled={!p.search.trim() || p.looking} onClick={p.onExactLookup} title="Look up an exact decimal key" className="text-xs">
            <ArrowRight size={14} /> {p.looking ? "Seeking…" : "Exact key"}
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {p.loading && !p.records.length ? (
          <p role="status" className="p-8 text-center text-sm text-muted-foreground">Loading records…</p>
        ) : !p.records.length ? (
          <div className="p-8 text-center">
            <FileText size={22} className="mx-auto text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">No records yet</p>
            <p className="mt-1 text-xs text-muted-foreground">Create a record to get started.</p>
          </div>
        ) : p.shownCount === 0 ? (
          <div className="p-8 text-center">
            <FileText size={22} className="mx-auto text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">No matching records</p>
            <p className="mt-1 text-xs text-muted-foreground">Try another partition or search.</p>
          </div>
        ) : (
          p.records.map((r) => {
            const meta = getPartitionMeta(r.prefix);
            const bgClass = COLOR_BG[meta.color] ?? "bg-slate-400";
            const isSelected = p.selected?.keyStr === r.keyStr;
            return (
              <button key={r.keyStr} type="button" onClick={() => p.onSelect(r)} aria-current={isSelected ? "true" : undefined}
                className={`flex w-full items-start gap-3 border-b border-border/70 px-4 py-3 text-left transition ${isSelected ? "bg-beast-lime/10 border-l-4 border-l-beast-lime shadow-sm" : "hover:bg-zinc-900/60"}`}>
                {r.coverUrl ? (
                  <img src={r.coverUrl} alt="" className="h-12 w-12 shrink-0 rounded object-cover border border-zinc-800" />
                ) : (
                  <div className={`flex h-12 w-12 shrink-0 items-center justify-center rounded text-lg font-bold text-white ${bgClass}`}>
                    {r.primaryLabel.charAt(0).toUpperCase()}
                  </div>
                )}
                <span className="min-w-0 flex-1">
                  <span className={`block truncate text-sm font-medium ${isSelected ? "text-white" : ""}`}>{r.primaryLabel}</span>
                  <span className="mt-0.5 block truncate text-xs text-zinc-400">{r.secondaryLabel || r.format.replace("_", " ")}</span>
                  <span className="mt-0.5 block truncate font-mono text-xs text-zinc-500">{r.keyStr}</span>
                </span>
              </button>
            );
          })
        )}
      </div>
      <div className="flex items-center justify-between gap-2 border-t px-4 py-3 text-xs text-muted-foreground">
        <span>{p.shownCount} of {p.records.length} loaded{p.hasMore ? " · more available" : ""}</span>
        {p.hasMore && (
          <Button variant="outline" size="sm" disabled={p.loadingMore} onClick={p.onLoadMore}>
            {p.loadingMore ? "Loading…" : "Load more"}
          </Button>
        )}
      </div>
    </div>
  );
}
