import { Folder, Trash2 } from "lucide-react";
import { getCollectionName, getPartitionMeta } from "../utils/key-decoder";
import { NewCollectionForm } from "./NewCollectionForm";

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
  canWrite?: boolean;
  onCreateCollection?: (name: string) => Promise<void>;
  onDropCollection?: (name: string) => Promise<void>;
  onNotice?: (message: string, error?: boolean) => void;
}

export function PartitionRail({ partitions, selected, total, hasMore, hasFullCounts, onSelect, canWrite, onCreateCollection, onDropCollection, onNotice }: Props) {
  const notice = onNotice ?? (() => {});
  const drop = async (name: string) => {
    try {
      await onDropCollection?.(name);
      notice(`Collection "${name}" dropped.`);
    } catch (e) {
      notice((e as Error).message, true);
    }
  };
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
          const collection = getCollectionName(prefix);
          return (
            <div key={prefix} className="group relative">
            <button
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
            {canWrite && collection && onDropCollection && (
              <button
                type="button"
                aria-label={`Drop collection ${collection}`}
                title={count > 0 ? "Delete the records first; a collection can only be dropped when empty." : `Drop collection ${collection}`}
                disabled={count > 0}
                onClick={() => void drop(collection)}
                className="absolute right-9 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground opacity-0 hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-0 disabled:group-hover:opacity-40"
              >
                <Trash2 size={13} />
              </button>
            )}
            </div>
          );
        })}
      </nav>
      {canWrite && onCreateCollection && (
        <div className="border-t border-border p-2">
          <NewCollectionForm onCreate={onCreateCollection} onNotice={notice} />
        </div>
      )}
      {!hasFullCounts && hasMore && (
        <p className="border-t border-border px-4 py-2 text-xs text-muted-foreground">More records may be available.</p>
      )}
    </aside>
  );
}
