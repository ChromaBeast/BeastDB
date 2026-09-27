"use client";
import { ArrowLeft, FileText, Plus } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { recordPreview } from "../utils/display";

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
  return (
    <div aria-label="Records" className="flex min-h-0 flex-col border-r border-border">
      <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <Button variant="ghost" size="icon" className="lg:hidden" aria-label="Back to collections" onClick={p.onBack}>
            <ArrowLeft size={16} />
          </Button>
          <h2 className="truncate text-sm font-semibold text-foreground" title={p.title}>{p.title}</h2>
        </div>
        {p.canWrite && (
          <Button size="sm" onClick={p.onNew} className="h-7 shrink-0 gap-1 text-xs">
            <Plus size={13} /> New
          </Button>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {p.loading && !p.records.length ? (
          <p role="status" className="p-8 text-center text-sm text-muted-foreground">Loading records…</p>
        ) : !p.records.length ? (
          <div className="p-8 text-center">
            <FileText size={22} className="mx-auto text-muted-foreground" />
            <p className="mt-3 text-sm font-medium text-foreground">No records here</p>
            <p className="mt-1 text-xs text-muted-foreground">Choose another collection or create a record.</p>
          </div>
        ) : (
          <div className="divide-y divide-border/60">
            {p.records.map((record) => {
              const selected = p.selected?.keyStr === record.keyStr;
              return (
                <button
                  key={record.keyStr}
                  type="button"
                  onClick={() => p.onSelect(record)}
                  aria-current={selected ? "true" : undefined}
                  className={`w-full border-l-2 px-4 py-3 text-left transition ${selected
                    ? "border-primary bg-primary/10"
                    : "border-transparent hover:bg-muted"}`}
                >
                  <span className={`block truncate text-sm font-medium ${selected ? "text-primary" : "text-foreground"}`} title={record.primaryLabel}>
                    {record.primaryLabel}
                  </span>
                  <span className="mt-1 block truncate text-xs text-muted-foreground" title={recordPreview(record)}>
                    {recordPreview(record)}
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </div>

      <div className="flex items-center justify-between gap-2 border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span>{p.shownCount} shown{p.hasMore ? " · more available" : ""}</span>
        {p.hasMore && (
          <Button variant="outline" size="sm" disabled={p.loadingMore} onClick={p.onLoadMore} className="h-7 text-xs">
            {p.loadingMore ? "Loading…" : "Load more"}
          </Button>
        )}
      </div>
    </div>
  );
}
