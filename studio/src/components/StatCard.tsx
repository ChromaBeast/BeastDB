import type { ReactNode } from "react";
import { Card } from "./ui/card";
import { cn } from "@/lib/utils";

const HINT_TONES = {
  muted: "text-muted-foreground",
  primary: "text-primary",
  success: "text-success",
  warning: "text-warning",
  info: "text-info",
} as const;

export type StatTone = keyof typeof HINT_TONES;

interface StatCardProps {
  label: string;
  value: string;
  hint?: ReactNode;
  tone?: StatTone;
}

/** Figma "Stat card": muted caption, large number, optional coloured hint line. */
export function StatCard({ label, value, hint, tone = "muted" }: StatCardProps) {
  return (
    <Card className="px-5 py-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-[28px] font-bold leading-tight tracking-tight text-foreground" title={value}>
        {value}
      </p>
      {hint && <p className={cn("mt-1 text-xs", HINT_TONES[tone])}>{hint}</p>}
    </Card>
  );
}

/** Responsive row of stat cards (2 up on small screens, `cols` up on wide ones). */
export function StatGrid({ cols = 4, children }: { cols?: 3 | 4; children: ReactNode }) {
  return (
    <div className={cn("grid grid-cols-2 gap-4", cols === 4 ? "lg:grid-cols-4" : "lg:grid-cols-3")}>
      {children}
    </div>
  );
}
