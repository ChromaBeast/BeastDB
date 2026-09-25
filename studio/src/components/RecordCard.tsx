"use client";

import React from "react";
import { Eye, Copy, Check, Star, Database } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes, formatKey } from "../utils/format";

interface RecordCardProps {
  record: UniversalRecord;
  onSelect: (record: UniversalRecord) => void;
  onCopyKey: (keyStr: string) => void;
  isCopied: boolean;
}

export const RecordCard: React.FC<RecordCardProps> = ({
  record,
  onSelect,
  onCopyKey,
  isCopied,
}) => {
  return (
    <div
      onClick={() => onSelect(record)}
      className="group relative flex flex-col justify-between overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] p-4 backdrop-blur-xl transition-all duration-300 hover:border-purple-500/40 hover:shadow-xl hover:shadow-purple-500/10 cursor-pointer"
    >
      <div>
        {/* Card Header Media / Gradient */}
        {record.coverUrl ? (
          <div className="relative -mx-4 -mt-4 mb-3 h-36 w-[calc(100%+2rem)] overflow-hidden bg-black/40">
            <img
              src={record.coverUrl}
              alt=""
              className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
              onError={(e) => { (e.target as HTMLElement).style.display = "none"; }}
            />
            <div className="absolute inset-0 bg-gradient-to-t from-[#0c101b] via-transparent to-transparent" />
            <div className="absolute top-2.5 left-2.5 flex items-center gap-1.5">
              <span className="rounded-md bg-black/60 backdrop-blur-md border border-white/10 px-2 py-0.5 text-[10px] font-mono text-purple-300 font-semibold">
                0x{record.prefix.toString(16).padStart(2, "0").toUpperCase()}
              </span>
              <span className="rounded-md bg-black/60 backdrop-blur-md border border-white/10 px-2 py-0.5 text-[10px] font-medium text-slate-300">
                {record.prefixLabel}
              </span>
            </div>
          </div>
        ) : (
          <div className="mb-3 flex items-center justify-between border-b border-white/[0.06] pb-2.5">
            <div className="flex items-center gap-2">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-purple-500/10 text-purple-400">
                <Database className="h-3.5 w-3.5" />
              </div>
              <span className="rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-mono text-slate-400">
                0x{record.prefix.toString(16).padStart(2, "0").toUpperCase()}
              </span>
            </div>
            <span className="rounded-md border border-white/10 bg-white/[0.03] px-2 py-0.5 text-[10px] uppercase font-medium text-slate-300">
              {record.format.replace("_", " ")}
            </span>
          </div>
        )}

        {/* Title & Secondary */}
        <div className="flex items-baseline justify-between gap-2">
          <h3 className="text-sm font-semibold text-white group-hover:text-purple-300 transition-colors truncate">
            {record.primaryLabel}
          </h3>
          {record.rating && (
            <span className="flex items-center gap-1 text-[11px] font-bold text-amber-400 shrink-0">
              <Star className="h-3 w-3 fill-amber-400" />
              {record.rating}
            </span>
          )}
        </div>

        {record.secondaryLabel && (
          <p className="mt-0.5 text-xs text-slate-400 line-clamp-1">{record.secondaryLabel}</p>
        )}

        {/* Dynamic Discovered Attributes Grid */}
        <div className="mt-3 flex flex-wrap gap-1.5">
          {record.attributes.slice(0, 4).map((attr) => (
            <div
              key={attr.key}
              className="flex items-center gap-1 rounded-md bg-black/30 border border-white/[0.06] px-2 py-0.5 text-[10px] text-slate-300"
            >
              <span className="text-slate-400">{attr.key}:</span>
              <span className="font-medium text-purple-300 truncate max-w-[110px]">{attr.value}</span>
            </div>
          ))}
          {record.status && (
            <span className="rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[10px] font-medium text-emerald-400">
              {record.status}
            </span>
          )}
        </div>
      </div>

      {/* Card Footer */}
      <div className="mt-4 flex items-center justify-between border-t border-white/[0.06] pt-3 text-[11px] text-slate-400">
        <div className="flex items-center gap-1.5 font-mono">
          <span>{formatKey(record.keyStr)}</span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onCopyKey(record.keyStr);
            }}
            className="text-slate-400 hover:text-white"
            title="Copy key"
          >
            {isCopied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          </button>
        </div>
        <div className="flex items-center gap-2">
          <span>{formatBytes(record.byteSize)}</span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onSelect(record);
            }}
            className="flex items-center gap-1 rounded-lg border border-white/10 bg-white/[0.04] px-2 py-1 text-[11px] text-purple-300 hover:bg-purple-600/30"
          >
            <Eye className="h-3 w-3" />
            <span>Inspect</span>
          </button>
        </div>
      </div>
    </div>
  );
};
