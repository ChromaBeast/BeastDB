"use client";
import { UniversalRecord } from "../types";
import { UserCollectionResult } from "../hooks/useUserContext";

const COLOR_TEXT: Record<string, string> = {
  emerald: "text-emerald-500", teal: "text-teal-500",
  purple: "text-purple-500", cyan: "text-cyan-500",
  amber: "text-amber-500", indigo: "text-indigo-500",
  rose: "text-rose-500", blue: "text-blue-500",
  violet: "text-violet-500", sky: "text-sky-500",
  slate: "text-slate-400",
};

interface Props {
  collection: UserCollectionResult;
  onSelect: (record: UniversalRecord) => void;
}

export function UserCollectionSection({ collection, onSelect }: Props) {
  const textColor = COLOR_TEXT[collection.color] ?? "text-muted-foreground";

  return (
    <details open className="group rounded-lg border border-border bg-card/50 text-sm">
      <summary className="flex cursor-pointer select-none items-center justify-between px-4 py-2.5 hover:bg-muted/50 rounded-lg">
        <span className={`font-medium ${textColor}`}>{collection.label}</span>
        <span className="ml-2 rounded-full bg-muted px-2 py-0.5 text-xs font-mono text-muted-foreground">
          {collection.records.length}
        </span>
      </summary>
      <ul className="divide-y divide-border/60 border-t border-border">
        {collection.records.map((r) => (
          <li key={r.keyStr}>
            <button
              type="button"
              onClick={() => onSelect(r)}
              className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-muted/50 transition"
            >
              {r.coverUrl && (
                <img src={r.coverUrl} alt="" className="h-8 w-8 shrink-0 rounded object-cover border border-border" />
              )}
              <div className="min-w-0 flex-1">
                <p className="truncate text-xs font-medium text-foreground">{r.primaryLabel}</p>
                {r.secondaryLabel && (
                  <p className="truncate text-[11px] text-muted-foreground">{r.secondaryLabel}</p>
                )}
              </div>
              {r.status && (
                <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground capitalize">
                  {r.status}
                </span>
              )}
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}
