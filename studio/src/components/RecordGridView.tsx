"use client";
import { FileText, Star } from "lucide-react";
import { UniversalRecord } from "../types";

interface Props {
  records: UniversalRecord[];
  selectedKey?: string;
  onSelect: (record: UniversalRecord) => void;
}

const STATUS_PILL: Record<string, string> = {
  completed: "bg-emerald-600 text-white",
  finished: "bg-emerald-600 text-white",
  playing: "bg-purple-600 text-white",
  watching: "bg-cyan-600 text-white",
  reading: "bg-rose-600 text-white",
  backlog: "bg-amber-600 text-white",
  active: "bg-blue-600 text-white",
};

export function RecordGridView({ records, selectedKey, onSelect }: Props) {
  if (records.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center p-12 text-center">
        <FileText size={28} className="text-muted-foreground/60" />
        <p className="mt-3 text-sm font-medium text-foreground">No records match filter</p>
        <p className="mt-1 text-xs text-muted-foreground">Select another filter or add records.</p>
      </div>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4 border-t border-border">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        {records.map((r) => {
          const isSelected = selectedKey === r.keyStr;
          const statusKey = String(r.status ?? "").toLowerCase();
          const pillClass = STATUS_PILL[statusKey] ?? "bg-zinc-800 text-zinc-200";

          return (
            <button
              key={r.keyStr}
              type="button"
              onClick={() => onSelect(r)}
              className={`group flex flex-col overflow-hidden rounded-xl border text-left transition duration-150 hover:shadow-md hover:border-primary/50 ${
                isSelected ? "border-primary ring-2 ring-primary/30 bg-primary/5" : "border-border bg-card"
              }`}
            >
              {/* Poster Thumbnail */}
              <div className="relative aspect-[3/4] w-full overflow-hidden bg-muted">
                {r.coverUrl ? (
                  <img
                    src={r.coverUrl}
                    alt=""
                    className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
                  />
                ) : (
                  <div className="flex h-full w-full flex-col items-center justify-center p-4 text-center text-muted-foreground">
                    <FileText size={24} className="opacity-50" />
                    <span className="mt-2 text-[10px] font-mono opacity-70 truncate max-w-full">
                      {r.prefixLabel}
                    </span>
                  </div>
                )}

                {/* Floating Status Pill */}
                {r.status && (
                  <span className={`absolute top-2 right-2 rounded-full px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wider shadow-sm ${pillClass}`}>
                    {r.status}
                  </span>
                )}

                {/* Floating Rating */}
                {r.rating != null && (
                  <span className="absolute bottom-2 left-2 flex items-center gap-1 rounded-md bg-black/75 px-1.5 py-0.5 text-[10px] font-bold text-amber-400 backdrop-blur-sm shadow-sm">
                    <Star size={10} className="fill-amber-400" />
                    {String(r.rating)}
                  </span>
                )}
              </div>

              {/* Card Meta */}
              <div className="flex flex-1 flex-col p-2.5">
                <p className="line-clamp-2 text-xs font-semibold text-foreground group-hover:text-primary transition">
                  {r.primaryLabel}
                </p>
                {r.secondaryLabel && (
                  <p className="mt-0.5 line-clamp-1 text-[10px] text-muted-foreground">
                    {r.secondaryLabel}
                  </p>
                )}
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
