const COLOR_HEX: Record<string, string> = {
  emerald: "#10b981", teal: "#14b8a6", purple: "#a855f7",
  cyan: "#06b6d4", amber: "#f59e0b", indigo: "#6366f1",
  rose: "#f43f5e", blue: "#3b82f6", violet: "#8b5cf6",
  sky: "#0ea5e9", fuchsia: "#d946ef", orange: "#f97316",
  lime: "#A8F21A", slate: "#71717a",
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

  const cx = 80; const cy = 80; const r = 70; const inner = 42;
  let currentAngle = 0;

  return (
    <div className="flex flex-wrap items-center gap-8">
      <div className="relative shrink-0">
        <svg width={160} height={160} aria-hidden="true">
          {data.map((d, i) => {
            const sweep = (d.count / total) * 360;
            const startAngle = currentAngle;
            const endAngle = currentAngle + sweep - 0.5;
            currentAngle += sweep;
            const hex = COLOR_HEX[d.color] ?? COLOR_HEX.slate;
            return <path key={i} d={describeArc(cx, cy, r, startAngle, Math.max(endAngle, startAngle + 0.1))} fill={hex} />;
          })}
          <circle cx={cx} cy={cy} r={inner} className="fill-[#09090b]" />
          <text x={cx} y={cy - 4} textAnchor="middle" className="fill-white font-mono text-base font-bold" fontSize={16} fontWeight={700}>
            {total.toLocaleString()}
          </text>
          <text x={cx} y={cy + 13} textAnchor="middle" fontSize={10} className="fill-zinc-500 font-mono uppercase tracking-wider">
            records
          </text>
        </svg>
      </div>
      <div className="grid flex-1 grid-cols-1 gap-2 sm:grid-cols-2">
        {data.map((d, i) => {
          const pct = ((d.count / total) * 100).toFixed(1);
          const colorHex = COLOR_HEX[d.color] ?? COLOR_HEX.slate;
          return (
            <div key={i} className="flex items-center justify-between gap-3 rounded-lg border border-zinc-800/70 bg-zinc-900/40 px-3 py-2 text-xs">
              <div className="flex min-w-0 items-center gap-2">
                <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: colorHex }} />
                <span className="truncate text-zinc-300 font-medium">{d.label}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="font-mono text-zinc-400 tabular-nums">{d.count.toLocaleString()}</span>
                <span className="rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-[10px] text-zinc-400">{pct}%</span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
