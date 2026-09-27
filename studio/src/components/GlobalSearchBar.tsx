"use client";
import { useState } from "react";
import { ArrowRight, Binary, FileSearch, Hash, Search } from "lucide-react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

export type SearchMode = "text" | "exact" | "range";

interface Props {
  mode: SearchMode;
  onModeChange: (mode: SearchMode) => void;
  partitions: Array<{ prefix: number; prefixLabel: string }>;
  activePartition: string;
  onSearchText: (query: string, prefix?: number) => void;
  onLookupKey: (key: string) => void;
  onScanRange: (startKey: string, endKey: string) => void;
  busy: boolean;
}

export function GlobalSearchBar({
  mode,
  onModeChange,
  partitions,
  activePartition,
  onSearchText,
  onLookupKey,
  onScanRange,
  busy,
}: Props) {
  const [textQuery, setTextQuery] = useState("");
  const [selectedPrefix, setSelectedPrefix] = useState(activePartition);
  const [exactKey, setExactKey] = useState("");
  const [rangeStart, setRangeStart] = useState("");
  const [rangeEnd, setRangeEnd] = useState("");

  const handleTextSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!textQuery.trim() || busy) return;
    const pfx = selectedPrefix !== "all" ? Number(selectedPrefix) : undefined;
    onSearchText(textQuery.trim(), pfx);
  };

  const handleExactSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!exactKey.trim() || busy) return;
    onLookupKey(exactKey.trim());
  };

  const handleRangeSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!rangeStart.trim() || !rangeEnd.trim() || busy) return;
    onScanRange(rangeStart.trim(), rangeEnd.trim());
  };

  return (
    <div className="rounded-xl border border-border bg-card p-3 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 pb-2.5">
        <div className="flex items-center gap-1 rounded-lg border border-border bg-zinc-900/60 p-0.5 text-xs">
          <button
            type="button"
            onClick={() => onModeChange("text")}
            className={`flex items-center gap-1.5 rounded-md px-3 py-1 font-medium transition ${
              mode === "text"
                ? "bg-beast-lime/15 text-beast-lime shadow-sm"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            <FileSearch size={13} /> Value Text
          </button>
          <button
            type="button"
            onClick={() => onModeChange("exact")}
            className={`flex items-center gap-1.5 rounded-md px-3 py-1 font-medium transition ${
              mode === "exact"
                ? "bg-beast-lime/15 text-beast-lime shadow-sm"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            <Hash size={13} /> Exact Key
          </button>
          <button
            type="button"
            onClick={() => onModeChange("range")}
            className={`flex items-center gap-1.5 rounded-md px-3 py-1 font-medium transition ${
              mode === "range"
                ? "bg-beast-lime/15 text-beast-lime shadow-sm"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            <Binary size={13} /> Key Range
          </button>
        </div>

        <span className="text-[11px] font-mono text-zinc-400">
          {mode === "text" && "Scans values inside B+ Tree slotted pages"}
          {mode === "exact" && "O(log N) point lookup in B+ Tree index"}
          {mode === "range" && "Sequential leaf-node range traversal"}
        </span>
      </div>

      <div className="pt-2.5">
        {mode === "text" && (
          <form onSubmit={handleTextSubmit} className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-0 flex-1">
              <Search size={15} className="absolute left-3 top-2.5 text-muted-foreground" />
              <Input
                value={textQuery}
                onChange={(e) => setTextQuery(e.target.value)}
                placeholder="Search record content across database (min 2 chars)..."
                className="pl-9 text-xs"
              />
            </div>
            <select
              aria-label="Filter search partition"
              value={selectedPrefix}
              onChange={(e) => setSelectedPrefix(e.target.value)}
              className="h-9 rounded-md border bg-card px-3 text-xs"
            >
              <option value="all">Entire Keyspace (All Partitions)</option>
              {partitions.map((p) => (
                <option key={p.prefix} value={p.prefix}>
                  {p.prefixLabel} · 0x{p.prefix.toString(16).padStart(2, "0").toUpperCase()}
                </option>
              ))}
            </select>
            <Button type="submit" disabled={textQuery.trim().length < 2 || busy} size="sm" className="gap-1.5 text-xs">
              <Search size={14} /> {busy ? "Scanning..." : "Search Database"}
            </Button>
          </form>
        )}

        {mode === "exact" && (
          <form onSubmit={handleExactSubmit} className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-0 flex-1">
              <Hash size={15} className="absolute left-3 top-2.5 text-muted-foreground" />
              <Input
                value={exactKey}
                onChange={(e) => setExactKey(e.target.value)}
                placeholder="Enter 64-bit decimal key (e.g. 72057594037927937)..."
                className="pl-9 font-mono text-xs"
              />
            </div>
            <Button type="submit" disabled={!exactKey.trim() || busy} size="sm" className="gap-1.5 text-xs">
              <ArrowRight size={14} /> {busy ? "Looking up..." : "Find Key"}
            </Button>
          </form>
        )}

        {mode === "range" && (
          <form onSubmit={handleRangeSubmit} className="flex flex-wrap items-center gap-2">
            <Input
              value={rangeStart}
              onChange={(e) => setRangeStart(e.target.value)}
              placeholder="Start Key (decimal)..."
              className="min-w-0 flex-1 font-mono text-xs"
            />
            <span className="text-xs text-zinc-500 font-mono">to</span>
            <Input
              value={rangeEnd}
              onChange={(e) => setRangeEnd(e.target.value)}
              placeholder="End Key (decimal)..."
              className="min-w-0 flex-1 font-mono text-xs"
            />
            <Button type="submit" disabled={!rangeStart.trim() || !rangeEnd.trim() || busy} size="sm" className="gap-1.5 text-xs">
              <Binary size={14} /> {busy ? "Scanning..." : "Scan Range"}
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}
