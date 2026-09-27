"use client";
import { useState, useMemo } from "react";
import { FileText, Star } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatKeyCompact } from "../utils/key-decoder";
import { PaginationBar } from "./PaginationBar";

interface Props {
  records: UniversalRecord[];
  selectedKey?: string;
  hasMore?: boolean;
  loadingMore?: boolean;
  onLoadMore?: () => void;
  onSelect: (record: UniversalRecord) => void;
}

const PAGE_SIZE = 25;

const STATUS_COLORS: Record<string, string> = {
  completed: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30",
  finished: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30",
  playing: "bg-purple-500/15 text-purple-600 dark:text-purple-400 border-purple-500/30",
  watching: "bg-cyan-500/15 text-cyan-600 dark:text-cyan-400 border-cyan-500/30",
  reading: "bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/30",
  backlog: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30",
  paused: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30",
  dropped: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/30",
  active: "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/30",
  admin: "bg-lime-500/15 text-lime-600 dark:text-lime-400 border-lime-500/30",
};

export function RecordTableView({
  records,
  selectedKey,
  hasMore,
  loadingMore,
  onLoadMore,
  onSelect,
}: Props) {
  const [page, setPage] = useState(1);

  const paginatedRecords = useMemo(() => {
    const start = (page - 1) * PAGE_SIZE;
    return records.slice(start, start + PAGE_SIZE);
  }, [records, page]);

  if (records.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center p-12 text-center">
        <FileText size={28} className="text-muted-foreground/60" />
        <p className="mt-3 text-sm font-medium text-foreground">No records match filter</p>
        <p className="mt-1 text-xs text-muted-foreground">Select another filter or add records.</p>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden border-t border-border">
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-collapse text-left text-xs">
          <thead className="sticky top-0 z-10 border-b border-border bg-muted/90 backdrop-blur-sm">
            <tr>
              <th className="w-12 px-3 py-2.5 font-medium text-muted-foreground">Cover</th>
              <th className="min-w-[200px] px-3 py-2.5 font-medium text-muted-foreground">Title / Identity</th>
              <th className="w-28 px-3 py-2.5 font-medium text-muted-foreground">Status</th>
              <th className="w-20 px-3 py-2.5 font-medium text-muted-foreground">Rating</th>
              <th className="px-3 py-2.5 font-medium text-muted-foreground">Attributes</th>
              <th className="w-28 px-3 py-2.5 text-right font-medium text-muted-foreground">Key</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {paginatedRecords.map((r) => {
              const isSelected = selectedKey === r.keyStr;
              const statusKey = String(r.status ?? "").toLowerCase();
              const badgeClass = STATUS_COLORS[statusKey] ?? "bg-muted text-muted-foreground border-border";

              return (
                <tr
                  key={r.keyStr}
                  onClick={() => onSelect(r)}
                  className={`cursor-pointer transition hover:bg-muted/60 ${
                    isSelected ? "bg-primary/10 font-medium" : ""
                  }`}
                >
                  <td className="px-3 py-2">
                    {r.coverUrl ? (
                      <img src={r.coverUrl} alt="" className="h-9 w-9 shrink-0 rounded object-cover border border-border" />
                    ) : (
                      <div className="flex h-9 w-9 items-center justify-center rounded bg-muted text-muted-foreground">
                        <FileText size={16} />
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <p className="truncate font-semibold text-foreground">{r.primaryLabel}</p>
                    {r.secondaryLabel && <p className="truncate text-[11px] text-muted-foreground">{r.secondaryLabel}</p>}
                  </td>
                  <td className="px-3 py-2">
                    {r.status ? (
                      <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium capitalize ${badgeClass}`}>
                        {r.status}
                      </span>
                    ) : (
                      <span className="text-muted-foreground/60">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    {r.rating != null ? (
                      <span className="inline-flex items-center gap-1 font-mono text-xs font-semibold text-amber-500">
                        <Star size={11} className="fill-amber-400" />
                        {String(r.rating)}
                      </span>
                    ) : (
                      <span className="text-muted-foreground/60">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                      {r.attributes.slice(0, 3).map((attr) => (
                        <span key={attr.key} className="rounded bg-muted/80 px-1.5 py-0.5 text-[10px] text-muted-foreground font-mono">
                          <span className="opacity-70">{attr.key.split(".").pop()}:</span> {attr.value}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-right font-mono text-[11px] text-muted-foreground">
                    {formatKeyCompact(r.keyStr)}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <PaginationBar
        page={page}
        pageSize={PAGE_SIZE}
        totalShown={records.length}
        hasMore={hasMore}
        loadingMore={loadingMore}
        onPageChange={setPage}
        onLoadMore={onLoadMore}
      />
    </div>
  );
}
