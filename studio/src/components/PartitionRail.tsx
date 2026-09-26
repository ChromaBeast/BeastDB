import { Database, Folder } from "lucide-react";
import { getPartitionMeta } from "../utils/key-decoder";

const COLOR_BORDER: Record<string, string> = {
  emerald: "border-emerald-500", teal: "border-teal-500",
  purple: "border-purple-500", cyan: "border-cyan-500",
  amber: "border-amber-500", indigo: "border-indigo-500",
  rose: "border-rose-500", blue: "border-blue-500",
  violet: "border-violet-500", sky: "border-sky-500",
  fuchsia: "border-fuchsia-500", orange: "border-orange-500",
  slate: "border-slate-400",
};

interface Props {
  partitions: Array<{ prefix: number; count: number; prefixLabel: string }>;
  selected: string;
  total: number;
  hasMore: boolean;
  hasFullCounts?: boolean;
  onSelect: (value: string) => void;
}

export function PartitionRail({ partitions, selected, total, hasMore, hasFullCounts, onSelect }: Props) {
  return (
    <aside aria-label="Partitions" className="flex min-h-0 flex-col border-r border-border">
      <div className="border-b border-border px-5 py-5">
        <p className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">BeastDB</p>
        <h2 className="mt-1 text-base font-semibold">Partitions</h2>
        <p className="mt-1 text-xs text-zinc-400">Keyspace segments (0x00 - 0xFF)</p>
      </div>
      <nav aria-label="Partition list" className="flex-1 space-y-0.5 overflow-y-auto p-2">
        <button
          type="button"
          onClick={() => onSelect("all")}
          aria-current={selected === "all" ? "page" : undefined}
          className={`flex w-full items-center gap-3 rounded-md border-l-4 px-3 py-2.5 text-left text-sm transition ${selected === "all" ? "border-l-beast-lime bg-beast-lime/10 text-white font-medium shadow-sm" : "border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900/60"}`}
        >
          <Database size={15} className={`shrink-0 ${selected === "all" ? "text-beast-lime" : "text-zinc-400"}`} />
          <span className="min-w-0 flex-1 truncate">All Records</span>
          <span className="text-xs font-mono tabular-nums text-zinc-500">{total}</span>
        </button>
        {partitions.map(({ prefix, count, prefixLabel }) => {
          const meta = getPartitionMeta(prefix);
          const borderClass = COLOR_BORDER[meta.color] ?? "border-zinc-500";
          const isSelected = selected === String(prefix);
          return (
            <button
              key={prefix}
              type="button"
              onClick={() => onSelect(String(prefix))}
              aria-current={isSelected ? "page" : undefined}
              className={`flex w-full items-center gap-3 rounded-md border-l-4 px-3 py-2.5 text-left text-sm transition ${isSelected ? `${borderClass} bg-beast-lime/10 text-white font-medium shadow-sm` : "border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900/60"}`}
            >
              <Folder size={15} className={`shrink-0 ${isSelected ? "text-beast-lime" : "text-zinc-400"}`} />
              <span className="min-w-0 flex-1 truncate" title={prefixLabel}>{prefixLabel}</span>
              <span className="text-xs font-mono tabular-nums text-zinc-500">{count}</span>
            </button>
          );
        })}
      </nav>
      <p className="border-t border-border px-4 py-3 text-[11px] font-mono text-zinc-500">
        {hasFullCounts
          ? "All registered partitions across keyspace."
          : `Counts reflect loaded records${hasMore ? "; more are available" : ""}.`}
      </p>
    </aside>
  );
}
