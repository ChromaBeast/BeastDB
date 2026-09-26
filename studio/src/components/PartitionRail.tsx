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
  onSelect: (value: string) => void;
}

export function PartitionRail({ partitions, selected, total, hasMore, onSelect }: Props) {
  return (
    <aside aria-label="Partitions" className="flex min-h-0 flex-col border-r">
      <div className="border-b px-5 py-5">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">BeastDB</p>
        <h2 className="mt-1 text-base font-semibold">Partitions</h2>
        <p className="mt-1 text-xs text-muted-foreground">Groups from record keys</p>
      </div>
      <nav aria-label="Partition list" className="flex-1 space-y-0.5 overflow-y-auto p-2">
        <button
          type="button"
          onClick={() => onSelect("all")}
          aria-current={selected === "all" ? "page" : undefined}
          className={`flex w-full items-center gap-3 rounded-md border-l-4 border-transparent px-3 py-2.5 text-left text-sm ${selected === "all" ? "border-l-4 border-primary bg-accent font-medium" : "hover:bg-accent/60"}`}
        >
          <Database size={15} className="shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">All Records</span>
          <span className="text-xs tabular-nums text-muted-foreground">{total}</span>
        </button>
        {partitions.map(({ prefix, count, prefixLabel }) => {
          const meta = getPartitionMeta(prefix);
          const borderClass = COLOR_BORDER[meta.color] ?? "border-slate-400";
          const isSelected = selected === String(prefix);
          return (
            <button
              key={prefix}
              type="button"
              onClick={() => onSelect(String(prefix))}
              aria-current={isSelected ? "page" : undefined}
              className={`flex w-full items-center gap-3 rounded-md border-l-4 px-3 py-2.5 text-left text-sm ${isSelected ? `${borderClass} bg-accent font-medium` : "border-transparent hover:bg-accent/60"}`}
            >
              <Folder size={15} className="shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate" title={prefixLabel}>{prefixLabel}</span>
              <span className="text-xs tabular-nums text-muted-foreground">{count}</span>
            </button>
          );
        })}
      </nav>
      <p className="border-t px-4 py-3 text-xs text-muted-foreground">
        Counts reflect loaded records{hasMore ? "; more are available" : ""}.
      </p>
    </aside>
  );
}
