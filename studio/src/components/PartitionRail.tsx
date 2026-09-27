import { Folder } from "lucide-react";
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
    <aside aria-label="Collections" className="flex min-h-0 flex-col border-r border-border">
      <div className="border-b border-border px-5 py-5">
        <h2 className="text-base font-semibold">Collections</h2>
        <p className="mt-1 text-xs text-muted-foreground">{total} records</p>
      </div>
      <nav aria-label="Collection list" className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
        {partitions.map(({ prefix, count, prefixLabel }) => {
          const meta = getPartitionMeta(prefix);
          const borderClass = COLOR_BORDER[meta.color] ?? "border-muted-foreground";
          const isSelected = selected === String(prefix);
          return (
            <button
              key={prefix}
              type="button"
              onClick={() => onSelect(String(prefix))}
              aria-current={isSelected ? "page" : undefined}
              className={`flex w-full items-center gap-2.5 rounded-md border-l-4 px-3 py-2.5 text-left text-sm transition ${
                isSelected
                  ? `${borderClass} bg-primary/10 text-foreground font-medium shadow-sm`
                  : "border-transparent text-muted-foreground hover:text-foreground hover:bg-muted"
              }`}
            >
              <Folder size={15} className={`shrink-0 ${isSelected ? "text-primary" : "text-muted-foreground"}`} />
              <span className="min-w-0 flex-1 truncate" title={prefixLabel}>{prefixLabel}</span>
              <span className="text-xs font-mono tabular-nums text-muted-foreground">{count}</span>
            </button>
          );
        })}
      </nav>
      {!hasFullCounts && hasMore && (
        <p className="border-t border-border px-4 py-2 text-xs text-muted-foreground">More records may be available.</p>
      )}
    </aside>
  );
}
