"use client";
import { useMemo } from "react";
import { UniversalRecord } from "../types";

interface Props {
  records: UniversalRecord[];
  activeStatus: string;
  onSelectStatus: (status: string) => void;
}

export function StatusFilterBar({ records, activeStatus, onSelectStatus }: Props) {
  const statusCounts = useMemo(() => {
    const map = new Map<string, number>();
    for (const r of records) {
      if (r.status && typeof r.status === "string") {
        const s = r.status.trim().toLowerCase();
        map.set(s, (map.get(s) ?? 0) + 1);
      }
    }
    return [...map.entries()].sort((a, b) => b[1] - a[1]);
  }, [records]);

  if (statusCounts.length === 0) return null;

  return (
    <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
      <button
        type="button"
        onClick={() => onSelectStatus("all")}
        className={`flex items-center gap-1.5 rounded-full px-3 py-1 font-medium transition ${
          activeStatus === "all"
            ? "bg-primary text-primary-foreground shadow-sm"
            : "bg-muted/80 text-muted-foreground hover:bg-muted hover:text-foreground"
        }`}
      >
        <span>All</span>
        <span className="font-mono text-[11px] opacity-80">{records.length}</span>
      </button>

      {statusCounts.map(([status, count]) => {
        const isSelected = activeStatus === status;
        return (
          <button
            key={status}
            type="button"
            onClick={() => onSelectStatus(isSelected ? "all" : status)}
            className={`flex items-center gap-1.5 rounded-full px-3 py-1 font-medium capitalize transition ${
              isSelected
                ? "bg-primary text-primary-foreground shadow-sm"
                : "bg-muted/80 text-muted-foreground hover:bg-muted hover:text-foreground"
            }`}
          >
            <span>{status}</span>
            <span className="font-mono text-[11px] opacity-80">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
