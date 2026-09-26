const COLOR_HEX: Record<string, string> = {
  emerald: "#10b981", teal: "#14b8a6", purple: "#a855f7",
  cyan: "#06b6d4", amber: "#f59e0b", indigo: "#6366f1",
  rose: "#f43f5e", blue: "#3b82f6", violet: "#8b5cf6",
  sky: "#0ea5e9", fuchsia: "#d946ef", orange: "#f97316",
  slate: "#94a3b8",
};

function describeArc(cx: number, cy: number, r: number, startAngle: number, endAngle: number): string {
  const toRad = (deg: number) => (deg * Math.PI) / 180;
  const x1 = cx + r * Math.cos(toRad(startAngle - 90));
  const y1 = cy + r * Math.sin(toRad(startAngle - 90));
  const x2 = cx + r * Math.cos(toRad(endAngle - 90));
  const y2 = cy + r * Math.sin(toRad(endAngle - 90));
  const large = endAngle - startAngle > 180 ? 1 : 0;
  return `M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`;
}

interface DataItem { label: string; count: number; color: string; }

export function PartitionChart({ data }: { data: DataItem[] }) {
  const total = data.reduce((sum, d) => sum + d.count, 0);
  if (total === 0) return null;

  const cx = 80; const cy = 80; const r = 70; const inner = 40;
  let currentAngle = 0;

  return (
    <div className="flex flex-wrap items-start gap-8">
      <div className="relative">
        <svg width={160} height={160} aria-hidden="true">
          {data.map((d, i) => {
            const sweep = (d.count / total) * 360;
            const startAngle = currentAngle;
            const endAngle = currentAngle + sweep - 0.5;
            currentAngle += sweep;
            const hex = COLOR_HEX[d.color] ?? COLOR_HEX.slate;
            return <path key={i} d={describeArc(cx, cy, r, startAngle, Math.max(endAngle, startAngle + 0.1))} fill={hex} />;
          })}
          {/* Inner hole */}
          <circle cx={cx} cy={cy} r={inner} className="fill-card" />
          <text x={cx} y={cy - 6} textAnchor="middle" className="fill-foreground text-lg font-bold" fontSize={18} fontWeight={700}>
            {total.toLocaleString()}
          </text>
          <text x={cx} y={cy + 12} textAnchor="middle" fontSize={10} className="fill-muted-foreground">
            total
          </text>
        </svg>
      </div>
      <ul className="flex flex-col gap-2 text-sm">
        {data.map((d, i) => (
          <li key={i} className="flex items-center gap-2">
            <span className="h-3 w-3 shrink-0 rounded-full" style={{ background: COLOR_HEX[d.color] ?? COLOR_HEX.slate }} />
            <span className="text-muted-foreground">{d.label}</span>
            <span className="ml-1 font-semibold tabular-nums">{d.count.toLocaleString()}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
