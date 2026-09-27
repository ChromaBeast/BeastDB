"use client";
import { useState } from "react";
import { ArrowLeft, FileText, Grid, List, Plus } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { getPartitionMeta } from "../utils/key-decoder";
import { formatBytes } from "../utils/format";
import { formatLabel, recordPreview } from "../utils/display";

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
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  title: string;
  shownCount: number;
  onLoadMore: () => void;
  onSelect: (r: UniversalRecord) => void;
  onBack: () => void;
  canWrite: boolean;
  onNew: () => void;
}

export function DocumentList(p: Props) {
  const [viewMode, setViewMode] = useState<"table" | "gallery">("table");

  return (
    <div aria-label="Documents" className="flex min-h-0 flex-col border-r border-border">
      {/* Header with Title & View Toggles */}
      <div className="flex items-center justify-between border-b border-border px-4 py-3.5">
        <div className="flex items-center gap-2 min-w-0">
          <Button variant="ghost" size="icon" className="lg:hidden" aria-label="Back to partitions" onClick={p.onBack}>
            <ArrowLeft size={16} />
          </Button>
          <div className="min-w-0">
            <h2 className="truncate font-semibold text-white text-sm" title={p.title}>{p.title}</h2>
            <p className="text-[11px] font-mono text-zinc-400">{p.records.length} loaded</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <div className="flex rounded-md border border-border p-0.5 bg-zinc-900/60">
            <Button
              size="icon"
              variant={viewMode === "table" ? "secondary" : "ghost"}
              onClick={() => setViewMode("table")}
              title="Compact Table View"
              className="h-6 w-6"
            >
              <List size={13} />
            </Button>
            <Button
              size="icon"
              variant={viewMode === "gallery" ? "secondary" : "ghost"}
              onClick={() => setViewMode("gallery")}
              title="Gallery Card View"
              className="h-6 w-6"
            >
              <Grid size={13} />
            </Button>
          </div>

          {p.canWrite && (
            <Button size="sm" onClick={p.onNew} className="h-7 gap-1 text-xs">
              <Plus size={13} /> New
            </Button>
          )}
        </div>
      </div>

      {/* Record Content Area */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {p.loading && !p.records.length ? (
          <p role="status" className="p-8 text-center text-xs text-muted-foreground">Loading partition records…</p>
        ) : !p.records.length ? (
          <div className="p-8 text-center">
            <FileText size={22} className="mx-auto text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">No records in partition</p>
            <p className="mt-1 text-xs text-muted-foreground">Create a record to get started.</p>
          </div>
        ) : viewMode === "table" ? (
          /* Compact Table View (Default) */
          <div className="divide-y divide-border/60">
            {p.records.map((r) => {
              const isSelected = p.selected?.keyStr === r.keyStr;
              return (
                <button
                  key={r.keyStr}
                  type="button"
                  onClick={() => p.onSelect(r)}
                  className={`flex w-full items-center gap-3 px-3.5 py-2 text-left text-xs transition ${
                    isSelected
                      ? "bg-beast-lime/10 border-l-2 border-l-beast-lime font-medium"
                      : "hover:bg-zinc-900/60"
                  }`}
                >
                  <span className="font-mono text-zinc-300 w-32 shrink-0 truncate" title={r.keyStr}>
                    {r.keyStr}
                  </span>
                  <Badge variant="mono" className="shrink-0 text-[10px] py-0 px-1.5">
                    {formatLabel(r.format)}
                  </Badge>
                  <span className="min-w-0 flex-1 truncate text-zinc-400">
                    {r.primaryLabel || recordPreview(r)}
                  </span>
                  <span className="font-mono text-[11px] text-zinc-500 shrink-0">
                    {formatBytes(r.byteSize)}
                  </span>
                </button>
              );
            })}
          </div>
        ) : (
          /* Visual Gallery View */
          <div className="divide-y divide-border/70">
            {p.records.map((r) => {
              const meta = getPartitionMeta(r.prefix);
              const bgClass = COLOR_BG[meta.color] ?? "bg-slate-400";
              const isSelected = p.selected?.keyStr === r.keyStr;
              return (
                <button
                  key={r.keyStr}
                  type="button"
                  onClick={() => p.onSelect(r)}
                  className={`flex w-full items-start gap-3 px-4 py-3 text-left transition ${
                    isSelected ? "bg-beast-lime/10 border-l-4 border-l-beast-lime shadow-sm" : "hover:bg-zinc-900/60"
                  }`}
                >
                  {r.coverUrl ? (
                    <img src={r.coverUrl} alt="" className="h-11 w-11 shrink-0 rounded object-cover border border-zinc-800" />
                  ) : (
                    <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded text-base font-bold text-white ${bgClass}`}>
                      {r.primaryLabel.charAt(0).toUpperCase()}
                    </div>
                  )}
                  <span className="min-w-0 flex-1">
                    <span className={`block truncate text-sm font-medium ${isSelected ? "text-white" : ""}`}>{r.primaryLabel}</span>
                    <span className="mt-0.5 block truncate text-xs text-zinc-400">{recordPreview(r)}</span>
                    <span className="mt-1 block text-[11px] font-mono text-zinc-500">{formatBytes(r.byteSize)} · {formatLabel(r.format)}</span>
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* Footer with pagination */}
      <div className="flex items-center justify-between gap-2 border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span>{p.records.length} loaded{p.hasMore ? " · more available" : ""}</span>
        {p.hasMore && (
          <Button variant="outline" size="sm" disabled={p.loadingMore} onClick={p.onLoadMore} className="h-7 text-xs">
            {p.loadingMore ? "Loading…" : "Load more"}
          </Button>
        )}
      </div>
    </div>
  );
}
