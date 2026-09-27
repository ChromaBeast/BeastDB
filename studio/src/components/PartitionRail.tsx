import { Folder, Key } from "lucide-react";
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
  onOpenTokens?: () => void;
}

export function PartitionRail({ partitions, selected, total, hasMore, hasFullCounts, onSelect, onOpenTokens }: Props) {
  return (
    <aside aria-label="Partitions" className="flex min-h-0 flex-col border-r border-border">
      <div className="border-b border-border px-5 py-5">
        <p className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">BeastDB</p>
        <h2 className="mt-1 text-base font-semibold">Partitions</h2>
        <p className="mt-1 text-xs text-zinc-400">Keyspace segments (0x00 - 0xFF)</p>
      </div>
      <nav aria-label="Partition list" className="flex-1 space-y-1 overflow-y-auto p-2">
        {partitions.map(({ prefix, count, prefixLabel }) => {
          const meta = getPartitionMeta(prefix);
          const borderClass = COLOR_BORDER[meta.color] ?? "border-zinc-500";
          const isSelected = selected === String(prefix);
          const hexPrefix = `0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`;
          return (
            <button
              key={prefix}
              type="button"
              onClick={() => onSelect(String(prefix))}
              aria-current={isSelected ? "page" : undefined}
              className={`flex w-full items-center gap-2.5 rounded-md border-l-4 px-3 py-2.5 text-left text-sm transition ${
                isSelected
                  ? `${borderClass} bg-beast-lime/10 text-white font-medium shadow-sm`
                  : "border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900/60"
              }`}
            >
              <span
                className={`h-2 w-2 shrink-0 rounded-full transition-colors ${
                  count > 0 ? "bg-beast-lime shadow-[0_0_6px_rgba(168,242,26,0.5)]" : "bg-zinc-600"
                }`}
                title={count > 0 ? `${count} records indexed` : "Empty partition"}
              />
              <Folder size={15} className={`shrink-0 ${isSelected ? "text-beast-lime" : "text-zinc-400"}`} />
              <span className="min-w-0 flex-1 truncate" title={prefixLabel}>{prefixLabel}</span>
              <span className="text-[10px] font-mono px-1 rounded bg-zinc-800/80 text-zinc-400">{hexPrefix}</span>
              <span className="text-xs font-mono tabular-nums text-zinc-500">{count}</span>
            </button>
          );
        })}
      </nav>
      {onOpenTokens && (
        <div className="border-t border-border p-2">
          <button
            type="button"
            onClick={onOpenTokens}
            className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900/60 transition group"
          >
            <Key size={15} className="shrink-0 text-amber-400 group-hover:scale-110 transition-transform" />
            <span className="min-w-0 flex-1 truncate font-medium">API Tokens</span>
            <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">0x05</span>
          </button>
        </div>
      )}
      <p className="border-t border-border px-4 py-3 text-[11px] font-mono text-zinc-500">
        {hasFullCounts
          ? "All registered partitions across keyspace."
          : `Counts reflect loaded records${hasMore ? "; more are available" : ""}.`}
      </p>
    </aside>
  );
}
