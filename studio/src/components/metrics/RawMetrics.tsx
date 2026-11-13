"use client";
import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { Card } from "../ui/card";
import type { Snapshot } from "../../utils/prometheus";

function fmt(v: number): string {
  if (!Number.isFinite(v)) return v > 0 ? "+Inf" : v < 0 ? "-Inf" : "NaN";
  return Number.isInteger(v) ? String(v) : v.toPrecision(6).replace(/\.?0+$/, "");
}

function labelText(l: Record<string, string>): string {
  const parts = Object.entries(l).map(([k, v]) => `${k}="${v}"`);
  return parts.length ? `{${parts.join(", ")}}` : "";
}

/** Collapsible list of every family and sample exactly as the endpoint exposed it. */
export function RawMetrics({ snapshot }: { snapshot: Snapshot }) {
  const [open, setOpen] = useState(false);
  const families = [...snapshot.values()].sort((a, b) => a.name.localeCompare(b.name));
  return (
    <Card>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 px-5 py-4 text-left focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ChevronRight size={14} className={`text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`} />
        <span className="text-sm font-semibold">Raw metrics</span>
        <span className="font-mono text-xs text-muted-foreground">{families.length} families</span>
      </button>
      {open && (
        <div className="max-h-[420px] overflow-auto border-t border-border px-5 py-3">
          {families.map((f) => (
            <div key={f.name} className="mb-3 last:mb-0">
              <div className="flex flex-wrap items-baseline gap-2">
                <code className="font-mono text-xs font-semibold text-foreground">{f.name}</code>
                <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">{f.type}</span>
              </div>
              {f.help && <p className="text-[11px] text-muted-foreground">{f.help}</p>}
              <ul className="mt-1 space-y-0.5">
                {f.samples.map((s, i) => (
                  <li key={i} className="flex justify-between gap-4 font-mono text-[11px] text-muted-foreground">
                    <span className="truncate" title={s.name + labelText(s.labels)}>
                      {s.name !== f.name ? s.name.slice(f.name.length) : ""}
                      {labelText(s.labels)}
                    </span>
                    <span className="shrink-0 text-foreground">{fmt(s.value)}</span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
