"use client";

import React, { useMemo } from "react";
import { UniversalRecord } from "../types";
import { Layers } from "lucide-react";

interface TypeDistributionBarProps {
  records: UniversalRecord[];
  activePrefixFilter: number | null;
  onSelectPrefix: (prefix: number | null) => void;
}

const COLOR_MAP: Record<string, string> = {
  emerald: "bg-emerald-500",
  teal: "bg-teal-500",
  purple: "bg-purple-500",
  cyan: "bg-cyan-500",
  amber: "bg-amber-500",
  indigo: "bg-indigo-500",
  rose: "bg-rose-500",
  blue: "bg-blue-500",
  violet: "bg-violet-500",
  sky: "bg-sky-500",
  fuchsia: "bg-fuchsia-500",
  slate: "bg-slate-500",
};

export const TypeDistributionBar: React.FC<TypeDistributionBarProps> = ({
  records,
  activePrefixFilter,
  onSelectPrefix,
}) => {
  const distribution = useMemo(() => {
    if (records.length === 0) return [];
    const counts: Record<number, { count: number; label: string }> = {};

    for (const r of records) {
      if (!counts[r.prefix]) {
        counts[r.prefix] = { count: 0, label: r.prefixLabel };
      }
      counts[r.prefix].count++;
    }

    return Object.entries(counts).map(([prefStr, item]) => {
      const prefix = Number(prefStr);
      const percent = (item.count / records.length) * 100;
      return {
        prefix,
        label: item.label,
        count: item.count,
        percent: Math.max(percent, 2),
      };
    });
  }, [records]);

  if (records.length === 0) return null;

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-4 backdrop-blur-xl">
      <div className="flex items-center justify-between pb-3">
        <div className="flex items-center gap-2">
          <Layers className="h-4 w-4 text-purple-400" />
          <span className="text-xs font-semibold text-white">Discovered Domain Partitions</span>
          <span className="text-[11px] text-slate-400 font-mono">({records.length} loaded records)</span>
        </div>
        {activePrefixFilter !== null && (
          <button
            onClick={() => onSelectPrefix(null)}
            className="text-[11px] font-medium text-purple-400 hover:text-purple-300"
          >
            Clear Partition Filter
          </button>
        )}
      </div>

      {/* Segmented Distribution Bar */}
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-black/40 gap-0.5 p-0.5">
        {distribution.map((d, idx) => {
          const isSelected = activePrefixFilter === d.prefix;
          const bgColors = [
            "bg-purple-500", "bg-cyan-500", "bg-emerald-500", "bg-amber-500",
            "bg-indigo-500", "bg-rose-500", "bg-teal-500", "bg-fuchsia-500",
          ];
          const colorClass = bgColors[idx % bgColors.length];

          return (
            <div
              key={d.prefix}
              onClick={() => onSelectPrefix(isSelected ? null : d.prefix)}
              style={{ width: `${d.percent}%` }}
              title={`${d.label}: ${d.count} records (${d.percent.toFixed(1)}%)`}
              className={`h-full cursor-pointer rounded-sm transition-all duration-200 hover:opacity-100 ${colorClass} ${
                activePrefixFilter !== null && !isSelected ? "opacity-30" : "opacity-85"
              }`}
            />
          );
        })}
      </div>

      {/* Pill Filter Badges */}
      <div className="mt-3 flex flex-wrap gap-1.5">
        <button
          onClick={() => onSelectPrefix(null)}
          className={`rounded-lg px-2.5 py-1 text-[11px] font-medium transition ${
            activePrefixFilter === null
              ? "bg-purple-600/30 text-purple-300 ring-1 ring-purple-500/50"
              : "text-slate-400 hover:bg-white/[0.04] hover:text-white"
          }`}
        >
          All Records ({records.length})
        </button>
        {distribution.map((d) => {
          const isSelected = activePrefixFilter === d.prefix;
          return (
            <button
              key={d.prefix}
              onClick={() => onSelectPrefix(isSelected ? null : d.prefix)}
              className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-[11px] font-medium transition ${
                isSelected
                  ? "bg-purple-600/30 text-purple-300 ring-1 ring-purple-500/50"
                  : "text-slate-400 hover:bg-white/[0.04] hover:text-white"
              }`}
            >
              <span className="font-mono opacity-60">0x{d.prefix.toString(16).padStart(2, "0").toUpperCase()}</span>
              <span>{d.label}</span>
              <span className="rounded bg-black/40 px-1.5 py-0.2 font-mono text-[10px] text-slate-300">{d.count}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
};
