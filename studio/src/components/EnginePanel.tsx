"use client";
import { useEffect, useRef, useState } from "react";
import { StorageMetrics } from "../types";
import { formatBytes } from "../utils/format";
import { cn } from "@/lib/utils";

type Cell = "pinned" | "dirty" | "clean" | "free";
const CELL_CLASS: Record<Cell, string> = {
  pinned: "bg-info",
  dirty: "bg-primary",
  clean: "bg-foreground/25",
  free: "bg-border/60",
};

/** Buffer-pool occupancy: poolSize cells ordered by state. Counts are real; cell order is not frame identity. */
export function BufferPoolMap({ storage }: { storage?: StorageMetrics }) {
  const pool = storage?.poolSize ?? 0;
  const pinned = Math.min(storage?.pinnedPages ?? 0, pool);
  const dirty = Math.min(storage?.dirtyPages ?? 0, pool - pinned);
  const cached = Math.min(storage?.cachedPages ?? 0, pool);
  const clean = Math.max(cached - pinned - dirty, 0);
  const cells: Cell[] = [
    ...Array<Cell>(pinned).fill("pinned"),
    ...Array<Cell>(dirty).fill("dirty"),
    ...Array<Cell>(clean).fill("clean"),
  ];
  while (cells.length < pool) cells.push("free");
  const legend: [Cell, string, number][] = [
    ["dirty", "Dirty", dirty],
    ["pinned", "Pinned", pinned],
    ["clean", "Cached", clean],
    ["free", "Free", pool - cells.filter((c) => c !== "free").length],
  ];
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold">Buffer pool</h2>
        <span className="font-mono text-xs text-muted-foreground">
          {cached}/{pool || "—"} frames · 4 KiB
        </span>
      </div>
      <div
        className="mt-4 grid gap-[3px]"
        style={{ gridTemplateColumns: "repeat(auto-fill, minmax(14px, 1fr))" }}
        role="img"
        aria-label={`${dirty} dirty, ${pinned} pinned, ${clean} clean cached, of ${pool} frames`}
      >
        {cells.map((c, i) => (
          <span key={i} className={cn("aspect-square rounded-[3px]", CELL_CLASS[c])} />
        ))}
      </div>
      <ul className="mt-4 flex flex-wrap gap-x-5 gap-y-1 font-mono text-xs text-muted-foreground">
        {legend.map(([c, label, n]) => (
          <li key={c} className="flex items-center gap-1.5">
            <span className={cn("h-2.5 w-2.5 rounded-[3px]", CELL_CLASS[c])} />
            {label} <span className="text-foreground">{n}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Write-ahead log head: LSN flashes accent when it advances between telemetry polls. */
export function WalHead({ lsn, storage }: { lsn?: number; storage?: StorageMetrics }) {
  const prev = useRef<number | undefined>(undefined);
  const [flash, setFlash] = useState(false);
  useEffect(() => {
    if (lsn !== undefined && prev.current !== undefined && lsn > prev.current) {
      setFlash(true);
      const t = window.setTimeout(() => setFlash(false), 900);
      prev.current = lsn;
      return () => clearTimeout(t);
    }
    prev.current = lsn;
  }, [lsn]);
  return (
    <div className="flex h-full flex-col">
      <h2 className="text-sm font-semibold">Write-ahead log</h2>
      <p className="mt-4 text-xs text-muted-foreground">Log sequence number</p>
      <p className={cn("mt-1 font-mono text-4xl font-bold tracking-tight transition-colors duration-700", flash ? "text-primary" : "text-foreground")}>
        {lsn?.toLocaleString() ?? "—"}
      </p>
      <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-border/60" aria-hidden="true">
        <div className={cn("h-full rounded-full bg-primary transition-all duration-700", flash ? "w-full" : "w-1/3")} />
      </div>
      <dl className="mt-auto grid grid-cols-2 gap-3 pt-5 font-mono text-xs">
        <div>
          <dt className="text-muted-foreground">WAL size</dt>
          <dd className="mt-0.5 text-sm text-foreground">{storage ? formatBytes(storage.walSizeBytes) : "—"}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Active page</dt>
          <dd className="mt-0.5 text-sm text-foreground">#{storage?.activeDataPage ?? "—"}</dd>
        </div>
      </dl>
    </div>
  );
}
