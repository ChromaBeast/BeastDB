"use client";
import { Badge } from "./ui/badge";
import { KeyStrip } from "./KeyStrip";
import { formatBytes } from "../utils/format";
import { decodeKey } from "../utils/key-decoder";
import type { QueryOutcome } from "./query-runner";

function preview(raw: string): string {
  const flat = raw.replace(/\s+/g, " ").trim();
  return flat.length > 120 ? `${flat.slice(0, 120)}…` : flat || "(empty)";
}

/** Results table + summary line for a finished query. */
export function QueryResults({ outcome, onContinue }: { outcome: QueryOutcome; onContinue?: (nextKey: string) => void }) {
  const { records, ms, scanned, indexed, hasMore, nextKey } = outcome;
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span><span className="font-mono text-foreground">{records.length}</span> {records.length === 1 ? "result" : "results"}</span>
        <span><span className="font-mono text-foreground">{ms.toFixed(1)}</span> ms</span>
        {scanned !== undefined && <span>scanned <span className="font-mono text-foreground">{scanned}</span></span>}
        {indexed && <Badge variant="mono">secondary index</Badge>}
        {hasMore && nextKey && onContinue && (
          <button
            type="button"
            onClick={() => onContinue(nextKey)}
            className="ml-auto font-mono text-primary hover:underline"
          >
            Continue from {nextKey} →
          </button>
        )}
      </div>
      {records.length === 0 ? (
        <p className="px-4 py-10 text-center text-sm text-muted-foreground">
          No records matched.{hasMore ? " More of the key space remains to be scanned." : ""}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-[11px] uppercase tracking-wide text-muted-foreground">
                <th className="px-4 py-2 font-medium">Key</th>
                <th className="px-4 py-2 font-medium">Partition</th>
                <th className="px-4 py-2 text-right font-medium">Size</th>
                <th className="px-4 py-2 font-medium">Preview</th>
              </tr>
            </thead>
            <tbody>
              {records.map((r) => {
                const d = decodeKey(r.keyStr);
                return (
                  <tr key={r.keyStr} className="border-b border-border/60 last:border-0 hover:bg-muted/40">
                    <td className="px-4 py-2.5">
                      <div className="font-mono text-xs text-foreground">{r.keyStr}</div>
                      <KeyStrip keyStr={r.keyStr} className="mt-1" />
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5">
                      <span className="text-foreground">{d.prefixLabel}</span>{" "}
                      <span className="font-mono text-xs text-muted-foreground">{d.prefixHex}</span>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-right font-mono text-xs">{formatBytes(r.byteSize)}</td>
                    <td className="max-w-md px-4 py-2.5">
                      <div className="truncate font-mono text-xs text-muted-foreground" title={r.raw}>{preview(r.raw)}</div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
