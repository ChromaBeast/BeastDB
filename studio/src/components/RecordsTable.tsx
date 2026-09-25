"use client";

import React from "react";
import { Eye, Trash2, Copy, Check, Database } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes, formatKey } from "../utils/format";

interface RecordsTableProps {
  records: UniversalRecord[];
  onSelect: (record: UniversalRecord) => void;
  onDelete: (key: number) => void;
  onCopyKey: (keyStr: string) => void;
  copiedKey: string | null;
}

export const RecordsTable: React.FC<RecordsTableProps> = ({
  records,
  onSelect,
  onDelete,
  onCopyKey,
  copiedKey,
}) => {
  return (
    <div className="overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] backdrop-blur-xl">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead className="border-b border-white/[0.06] bg-white/[0.01] text-slate-400">
            <tr>
              <th className="px-5 py-3.5 font-semibold uppercase tracking-wider">64-bit Key / Partition</th>
              <th className="px-5 py-3.5 font-semibold uppercase tracking-wider">Format</th>
              <th className="px-5 py-3.5 font-semibold uppercase tracking-wider">Primary Identifier</th>
              <th className="px-5 py-3.5 font-semibold uppercase tracking-wider">Discovered Attributes</th>
              <th className="px-5 py-3.5 font-semibold uppercase tracking-wider">Size</th>
              <th className="px-5 py-3.5 text-right font-semibold uppercase tracking-wider">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/[0.04]">
            {records.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-400">
                  <Database className="mx-auto mb-2 h-8 w-8 text-slate-600 opacity-60" />
                  No records match your query or partition filters.
                </td>
              </tr>
            ) : (
              records.map((rec) => {
                const isCopied = copiedKey === rec.keyStr;
                return (
                  <tr
                    key={rec.keyStr}
                    onClick={() => onSelect(rec)}
                    className="group cursor-pointer transition-colors hover:bg-purple-500/[0.04]"
                  >
                    {/* Key & Partition */}
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-purple-300 font-semibold">
                          {formatKey(rec.keyStr)}
                        </span>
                        <span className="rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-mono text-slate-400">
                          0x{rec.prefix.toString(16).padStart(2, "0").toUpperCase()}
                        </span>
                      </div>
                      <div className="mt-0.5 text-[10px] text-slate-400">{rec.prefixLabel}</div>
                    </td>

                    {/* Format Badge */}
                    <td className="px-5 py-3.5">
                      <span className="inline-flex items-center rounded-md border border-white/10 bg-white/[0.03] px-2 py-0.5 text-[10px] font-medium uppercase text-slate-300">
                        {rec.format.replace("_", " ")}
                      </span>
                    </td>

                    {/* Primary Identifier */}
                    <td className="px-5 py-3.5 max-w-xs truncate text-slate-200">
                      <div className="flex items-center gap-2.5">
                        {rec.coverUrl && (
                          <img
                            src={rec.coverUrl}
                            alt=""
                            className="h-7 w-7 rounded object-cover ring-1 ring-white/10 shrink-0"
                            onError={(e) => { (e.target as HTMLElement).style.display = "none"; }}
                          />
                        )}
                        <div className="truncate">
                          <div className="truncate font-medium text-white">{rec.primaryLabel}</div>
                          {rec.secondaryLabel && (
                            <div className="truncate text-[10px] text-slate-400">{rec.secondaryLabel}</div>
                          )}
                        </div>
                      </div>
                    </td>

                    {/* Discovered Attributes */}
                    <td className="px-5 py-3.5">
                      <div className="flex flex-wrap gap-1 max-w-sm">
                        {rec.attributes.slice(0, 3).map((attr) => (
                          <span
                            key={attr.key}
                            className="inline-flex items-center gap-1 rounded bg-black/40 border border-white/[0.06] px-1.5 py-0.5 text-[10px] text-slate-300"
                          >
                            <span className="text-slate-400">{attr.key}:</span>
                            <span className="font-medium text-purple-300 truncate max-w-[90px]">{attr.value}</span>
                          </span>
                        ))}
                        {rec.status && (
                          <span className="rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[10px] font-medium text-emerald-400">
                            {rec.status}
                          </span>
                        )}
                      </div>
                    </td>

                    {/* Size */}
                    <td className="px-5 py-3.5 font-mono text-[11px] text-slate-400">
                      {formatBytes(rec.byteSize)}
                    </td>

                    {/* Actions */}
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
                        <button
                          onClick={() => onCopyKey(rec.keyStr)}
                          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-white/10 hover:text-white"
                          title="Copy 64-bit key"
                        >
                          {isCopied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                        </button>
                        <button
                          onClick={() => onSelect(rec)}
                          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-white/10 hover:text-white"
                          title="Inspect record fields"
                        >
                          <Eye className="h-3.5 w-3.5" />
                        </button>
                        <button
                          onClick={() => onDelete(rec.key)}
                          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-red-500/10 hover:text-red-400"
                          title="Delete record"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      <div className="border-t border-white/[0.06] px-5 py-3 text-slate-400 text-xs flex justify-between items-center">
        <span>Showing {records.length} records</span>
        <span className="font-mono text-[11px] text-slate-400">BeastDB B+ Tree Storage</span>
      </div>
    </div>
  );
};
