import { useId } from "react";
import { cn } from "@/lib/utils";

interface Props {
  /** Values oldest to newest; undefined entries are gaps. */
  values: (number | undefined)[];
  /** Text alternative describing the series for assistive tech. */
  label: string;
  className?: string;
  /** Tailwind text color class; the stroke and fill use currentColor. */
  tone?: string;
  height?: number;
  /** Fix the y-axis floor to zero (default) or fit to data range. */
  zeroBased?: boolean;
}

const W = 240;

/** Hand-rolled area sparkline. Gaps break the line; the last point is marked. */
export function Sparkline({ values, label, className, tone = "text-primary", height = 56, zeroBased = true }: Props) {
  const gid = useId();
  const nums = values.filter((v): v is number => v !== undefined && Number.isFinite(v));
  if (nums.length < 2) {
    return (
      <div
        role="img"
        aria-label={`${label}: collecting samples`}
        className={cn("flex items-center justify-center rounded-md border border-dashed border-border text-[11px] text-muted-foreground", className)}
        style={{ height }}
      >
        collecting samples
      </div>
    );
  }
  const max = Math.max(...nums);
  const min = zeroBased ? 0 : Math.min(...nums);
  const span = max - min || 1;
  const pad = 3;
  const n = values.length;
  const x = (i: number) => (n === 1 ? 0 : (i / (n - 1)) * W);
  const y = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2);

  const runs: [number, number][][] = [];
  let cur: [number, number][] = [];
  values.forEach((v, i) => {
    if (v === undefined || !Number.isFinite(v)) {
      if (cur.length) runs.push(cur);
      cur = [];
    } else cur.push([x(i), y(v)]);
  });
  if (cur.length) runs.push(cur);

  const lastIdx = (() => {
    for (let i = values.length - 1; i >= 0; i--) if (values[i] !== undefined) return i;
    return -1;
  })();
  const last = lastIdx >= 0 ? values[lastIdx] : undefined;

  return (
    <svg
      role="img"
      aria-label={`${label}: latest ${last === undefined ? "n/a" : last.toPrecision(3)}, range ${min.toPrecision(3)} to ${max.toPrecision(3)}`}
      viewBox={`0 0 ${W} ${height}`}
      preserveAspectRatio="none"
      className={cn("w-full", tone, className)}
      style={{ height }}
    >
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.22" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <line x1="0" x2={W} y1={height - pad} y2={height - pad} className="stroke-border" strokeWidth="1" vectorEffect="non-scaling-stroke" />
      {runs.map((r, i) => {
        const line = r.map(([px, py], j) => `${j ? "L" : "M"}${px.toFixed(1)} ${py.toFixed(1)}`).join(" ");
        const area = `${line} L${r[r.length - 1][0].toFixed(1)} ${height - pad} L${r[0][0].toFixed(1)} ${height - pad} Z`;
        return (
          <g key={i}>
            {r.length > 1 && <path d={area} fill={`url(#${gid})`} />}
            <path d={line} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          </g>
        );
      })}
    </svg>
  );
}
