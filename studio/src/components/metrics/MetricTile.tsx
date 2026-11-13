import type { ReactNode } from "react";
import { Card } from "../ui/card";
import { Sparkline } from "./Sparkline";
import { cn } from "@/lib/utils";

interface Props {
  label: string;
  value: string;
  unit?: string;
  hint?: ReactNode;
  /** Optional history for a sparkline under the number. */
  series?: (number | undefined)[];
  tone?: string;
  /** Marks the value as a live/active state (lime). */
  live?: boolean;
}

/** Stat tile: caption, mono number, optional sparkline. */
export function MetricTile({ label, value, unit, hint, series, tone, live }: Props) {
  return (
    <Card className="p-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={cn("mt-1 font-mono text-2xl font-semibold tabular-nums", live ? "text-primary" : "text-foreground")}>
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-muted-foreground">{unit}</span>}
      </p>
      {hint && <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">{hint}</p>}
      {series && series.some((v) => v !== undefined) && (
        <Sparkline values={series} label={label} tone={tone} height={40} className="mt-3" />
      )}
    </Card>
  );
}
