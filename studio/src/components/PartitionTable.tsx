"use client";
import { Card } from "./ui/card";

export interface PartitionRow {
  prefix: number;
  label: string;
  count: number;
}

/** Per-partition key-prefix table: prefix hex, label, record count and share of all records. */
export function PartitionTable({ rows }: { rows: PartitionRow[] }) {
  const total = rows.reduce((s, r) => s + r.count, 0);
  return (
    <Card className="overflow-hidden">
      <div className="border-b border-border px-5 py-3.5">
        <h2 className="text-sm font-semibold text-foreground">Key-space partitions</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">The top 8 bits of each key select the partition; all partitions share one primary B+ tree.</p>
      </div>
      {rows.length === 0 ? (
        <p className="px-5 py-8 text-center text-sm text-muted-foreground">No partition data yet.</p>
      ) : (
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-border text-[11px] uppercase tracking-wide text-muted-foreground">
              <th className="px-5 py-2 font-medium">Prefix</th>
              <th className="px-5 py-2 font-medium">Label</th>
              <th className="px-5 py-2 text-right font-medium">Records</th>
              <th className="w-[35%] px-5 py-2 font-medium">Share</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const pct = total > 0 ? (r.count / total) * 100 : 0;
              return (
                <tr key={r.prefix} className="border-b border-border/60 last:border-0">
                  <td className="px-5 py-2.5 font-mono text-xs">0x{r.prefix.toString(16).padStart(2, "0").toUpperCase()}</td>
                  <td className="px-5 py-2.5 text-foreground">{r.label}</td>
                  <td className="px-5 py-2.5 text-right font-mono text-xs">{r.count.toLocaleString()}</td>
                  <td className="px-5 py-2.5">
                    <div className="flex items-center gap-2">
                      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full bg-foreground/50" style={{ width: `${pct}%` }} />
                      </div>
                      <span className="w-12 text-right font-mono text-xs text-muted-foreground">{pct.toFixed(1)}%</span>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Card>
  );
}
