"use client";

import React from "react";
import { Search, LayoutGrid, Table, Plus, ChevronRight, Filter } from "lucide-react";
import { ViewMode, PayloadFormat } from "../types";

interface RecordsToolbarProps {
  search: string;
  setSearch: (s: string) => void;
  formatFilter: PayloadFormat | "all";
  setFormatFilter: (f: PayloadFormat | "all") => void;
  viewMode: ViewMode;
  setViewMode: (v: ViewMode) => void;
  onOpenNew: () => void;
  hasMore: boolean;
  onLoadMore: () => void;
  isLoadingMore: boolean;
}

export const RecordsToolbar: React.FC<RecordsToolbarProps> = ({
  search,
  setSearch,
  formatFilter,
  setFormatFilter,
  viewMode,
  setViewMode,
  onOpenNew,
  hasMore,
  onLoadMore,
  isLoadingMore,
}) => {
  const formats: { id: PayloadFormat | "all"; label: string }[] = [
    { id: "all", label: "All Formats" },
    { id: "json_object", label: "JSON Docs" },
    { id: "string", label: "Strings / Indices" },
    { id: "token", label: "Tokens" },
    { id: "json_array", label: "Arrays" },
  ];

  return (
    <div className="flex flex-col gap-3 rounded-2xl border border-white/[0.08] bg-white/[0.02] p-4 backdrop-blur-xl sm:flex-row sm:items-center sm:justify-between">
      {/* Search Input & Format Filter */}
      <div className="flex flex-1 flex-wrap items-center gap-2">
        <div className="relative flex-1 min-w-[200px] sm:max-w-xs">
          <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search any key, field, or text..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-xl border border-white/10 bg-black/40 py-1.5 pl-9 pr-3 text-xs text-white placeholder-slate-500 outline-none transition focus:border-purple-500/50 focus:ring-1 focus:ring-purple-500/50"
          />
        </div>

        {/* Format Filter Dropdown / Pills */}
        <div className="flex items-center gap-1 rounded-xl border border-white/10 bg-black/30 p-1">
          {formats.map((f) => (
            <button
              key={f.id}
              onClick={() => setFormatFilter(f.id)}
              className={`rounded-lg px-2.5 py-1 text-[11px] font-medium transition ${
                formatFilter === f.id
                  ? "bg-purple-600/30 text-purple-300"
                  : "text-slate-400 hover:text-white"
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* View Switcher, Load More & New Record Button */}
      <div className="flex items-center gap-2 justify-end">
        {hasMore && (
          <button
            onClick={onLoadMore}
            disabled={isLoadingMore}
            className="flex items-center gap-1 rounded-xl border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs font-medium text-purple-300 hover:bg-white/[0.08] disabled:opacity-50"
          >
            <span>{isLoadingMore ? "Loading..." : "Load More"}</span>
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        )}

        {/* View Mode Toggle */}
        <div className="flex items-center rounded-xl border border-white/10 bg-black/40 p-1">
          <button
            onClick={() => setViewMode("table")}
            className={`rounded-lg p-1.5 transition ${
              viewMode === "table" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
            title="Data Table View"
          >
            <Table className="h-4 w-4" />
          </button>
          <button
            onClick={() => setViewMode("grid")}
            className={`rounded-lg p-1.5 transition ${
              viewMode === "grid" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
            title="Card Gallery View"
          >
            <LayoutGrid className="h-4 w-4" />
          </button>
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
  );
};
