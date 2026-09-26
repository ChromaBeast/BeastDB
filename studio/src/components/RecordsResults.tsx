"use client";
import { Copy } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";

interface Props {
  records: UniversalRecord[];
  view: "table" | "grid";
  onSelect: (r: UniversalRecord) => void;
  onNotice: (message: string, error?: boolean) => void;
}
function preview(record: UniversalRecord): string {
  if (record.secondaryLabel) return record.secondaryLabel;
  if (record.fields) {
    const summary = record.attributes
      .filter((field) => !["name", "title"].includes(field.key))
      .slice(0, 3)
      .map((field) => `${field.key}: ${field.value}`)
      .join(" · ");
    if (summary) return summary;
  }
  return record.raw || "Empty value";
}
export function RecordsResults({ records, view, onSelect, onNotice }: Props) {
  const copy = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key);
      onNotice("Key copied.");
    } catch {
      onNotice("Could not copy the key.", true);
    }
  };
  const cards = (
    <div
      className={`grid gap-3 sm:grid-cols-2 xl:grid-cols-3 ${view === "table" ? "md:hidden" : ""}`}
    >
      {records.map((r) => (
        <button
          key={r.keyStr}
          onClick={() => onSelect(r)}
          className="min-w-0 rounded-lg border bg-card p-5 text-left shadow-sm hover:border-foreground/30 hover:bg-accent/30"
        >
          <div className="flex items-start justify-between gap-2">
            <span className="truncate font-medium">{r.primaryLabel}</span>
            <span className="rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">
              {r.format.replace("_", " ")}
            </span>
          </div>
          <p className="mt-2 line-clamp-2 min-h-10 break-all text-sm text-muted-foreground">
            {preview(r)}
          </p>
          <div className="mt-4 flex justify-between gap-2 border-t pt-3 text-xs text-muted-foreground">
            <span className="truncate">{r.prefixLabel}</span>
            <span className="truncate font-mono">{r.keyStr}</span>
          </div>
        </button>
      ))}
    </div>
  );
  if (view === "grid") return cards;
  return (
    <>
      {cards}
      <div className="hidden overflow-hidden rounded-lg border bg-card shadow-sm md:block">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="bg-muted/60 text-xs text-muted-foreground">
              <tr>
                <th className="px-5 py-3 font-medium">Record</th>
                <th className="px-4 py-3 font-medium">Type</th>
                <th className="px-4 py-3 font-medium">Partition</th>
                <th className="px-4 py-3 font-medium">Key</th>
                <th className="px-4 py-3 font-medium">Size</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {records.map((r) => (
                <tr key={r.keyStr} className="hover:bg-accent/50">
                  <td className="max-w-[360px] px-5 py-3">
                    <button
                      className="block max-w-full truncate text-left font-medium hover:underline"
                      onClick={() => onSelect(r)}
                    >
                      {r.primaryLabel}
                    </button>
                    <span className="block max-w-full truncate text-xs text-muted-foreground">
                      {preview(r)}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {r.format.replace("_", " ")}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {r.prefixLabel}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs" title={r.keyStr}>
                    {r.keyStr}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {formatBytes(r.byteSize)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={`Copy key ${r.keyStr}`}
                      onClick={() => void copy(r.keyStr)}
                    >
                      <Copy size={15} />
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onSelect(r)}
                    >
                      Inspect
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}
