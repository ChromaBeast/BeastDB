"use client";

import React from "react";
import { Search, Plus, Eye, Trash2, Database, Film, Gamepad2, Tv, BookOpen, Layers } from "lucide-react";
import { DbRecord, CategoryFilter } from "../types";
import { formatKey } from "../utils/format";

interface RecordsTableProps {
  records: DbRecord[];
  filter: CategoryFilter;
  setFilter: (f: CategoryFilter) => void;
  search: string;
  setSearch: (s: string) => void;
  onSelect: (record: DbRecord) => void;
  onDelete: (key: number) => void;
  onOpenNew: () => void;
}

export const RecordsTable: React.FC<RecordsTableProps> = ({
  records, filter, setFilter, search, setSearch, onSelect, onDelete, onOpenNew,
}) => {
  const categories: { id: CategoryFilter; label: string; icon: React.ReactNode }[] = [
    { id: "all", label: "All Records", icon: <Layers className="h-3.5 w-3.5" /> },
    { id: "game", label: "Games", icon: <Gamepad2 className="h-3.5 w-3.5" /> },
    { id: "movie", label: "Movies", icon: <Film className="h-3.5 w-3.5" /> },
    { id: "tv", label: "TV Shows", icon: <Tv className="h-3.5 w-3.5" /> },
    { id: "book", label: "Books", icon: <BookOpen className="h-3.5 w-3.5" /> },
    { id: "system", label: "System", icon: <Database className="h-3.5 w-3.5" /> },
  ];

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] backdrop-blur-xl">
      {/* Table Toolbar */}
      <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between border-b border-white/[0.06]">
        {/* Category Pills */}
        <div className="flex flex-wrap items-center gap-1.5">
          {categories.map((cat) => (
            <button
              key={cat.id}
              onClick={() => setFilter(cat.id)}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-all ${
                filter === cat.id
                  ? "bg-purple-600/30 text-purple-300 ring-1 ring-purple-500/50"
                  : "text-slate-400 hover:bg-white/[0.04] hover:text-white"
              }`}
            >
              {cat.icon}
              {cat.label}
            </button>
          ))}
        </div>

        {/* Search & New Record */}
        <div className="flex items-center gap-2">
          <div className="relative flex-1 sm:w-64">
            <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              type="text"
              placeholder="Search keys or data..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-xl border border-white/10 bg-black/40 py-1.5 pl-9 pr-3 text-xs text-white placeholder-slate-500 outline-none transition focus:border-purple-500/50 focus:ring-1 focus:ring-purple-500/50"
            />
          </div>
          <button
            onClick={onOpenNew}
            className="flex items-center gap-1.5 rounded-xl bg-purple-600 px-3.5 py-1.5 text-xs font-semibold text-white shadow-lg shadow-purple-600/30 transition hover:bg-purple-500"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>New Record</span>
          </button>
        </div>
      </div>

      {/* Table List */}
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead className="border-b border-white/[0.06] bg-white/[0.01] text-slate-400">
            <tr>
              <th className="px-5 py-3 font-semibold uppercase tracking-wider">Key (64-bit)</th>
              <th className="px-5 py-3 font-semibold uppercase tracking-wider">Entity Type</th>
              <th className="px-5 py-3 font-semibold uppercase tracking-wider">Title / Preview</th>
              <th className="px-5 py-3 font-semibold uppercase tracking-wider">Status</th>
              <th className="px-5 py-3 text-right font-semibold uppercase tracking-wider">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/[0.04]">
            {records.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-400">
                  <Database className="mx-auto mb-2 h-8 w-8 text-slate-600 opacity-60" />
                  No records match your query or category filter.
                </td>
              </tr>
            ) : (
              records.map((rec) => {
                const type = rec.parsed?.type || (rec.value.startsWith("_sys") ? "system" : "custom");
                const title = rec.parsed?.title || rec.parsed?.name || rec.value.slice(0, 45);
                const status = rec.parsed?.status || (rec.parsed?.rating ? `${rec.parsed.rating} ★` : "Active");

                return (
                  <tr
                    key={rec.key}
                    onClick={() => onSelect(rec)}
                    className="group cursor-pointer transition-colors hover:bg-purple-500/[0.04]"
                  >
                    <td className="px-5 py-3.5 font-mono text-purple-300 font-semibold">
                      {formatKey(rec.key)}
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="inline-flex items-center rounded-md border border-purple-500/20 bg-purple-500/10 px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide text-purple-300">
                        {type}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 max-w-xs truncate font-medium text-slate-200">
                      {title}
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="inline-flex items-center rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-[11px] font-medium text-emerald-400 border border-emerald-500/20">
                        {status}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
                        <button
                          onClick={() => onSelect(rec)}
                          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-white/10 hover:text-white"
                          title="Inspect record"
                        >
                          <Eye className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => onDelete(rec.key)}
                          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-red-500/10 hover:text-red-400"
                          title="Delete record"
                        >
                          <Trash2 className="h-4 w-4" />
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

      {/* Footer Count */}
      <div className="border-t border-white/[0.06] px-5 py-3 text-slate-400 text-xs flex justify-between items-center">
        <span>Showing {records.length} records</span>
        <span className="font-mono text-[11px] text-slate-400">BeastDB B+ Tree Storage</span>
      </div>
    </div>
  );
};
