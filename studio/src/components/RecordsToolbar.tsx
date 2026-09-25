"use client";
import { ArrowRight, Grid2X2, List, Search } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

interface Props {
  records: UniversalRecord[];
  hasMore: boolean;
  search: string;
  setSearch: (v: string) => void;
  prefix: string;
  setPrefix: (v: string) => void;
  format: string;
  setFormat: (v: string) => void;
  view: "table" | "grid";
  setView: (v: "table" | "grid") => void;
  looking: boolean;
  onLookup: () => void;
}
export function RecordsToolbar(p: Props) {
  const partitions = Array.from(
    new Map(p.records.map((r) => [r.prefix, r.prefixLabel])).entries(),
  );
  const formats = Array.from(new Set(p.records.map((r) => r.format)));
  return (
    <div className="rounded-lg border bg-card p-4 shadow-sm">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div className="relative min-w-0 flex-1">
          <Search
            size={16}
            className="absolute left-3 top-2.5 text-muted-foreground"
          />
          <Input
            aria-label="Search loaded records"
            value={p.search}
            onChange={(e) => p.setSearch(e.target.value)}
            placeholder="Search loaded records or enter an exact key"
            className="pl-9"
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            onClick={p.onLookup}
            disabled={!p.search.trim() || p.looking}
            title="Look up the exact decimal key across the database"
          >
            <ArrowRight size={15} />
            {p.looking ? "Looking up" : "Find exact key"}
          </Button>
          <select
            aria-label="Filter by partition"
            className="h-9 max-w-44 rounded-md border bg-card px-3 text-sm"
            value={p.prefix}
            onChange={(e) => p.setPrefix(e.target.value)}
          >
            <option value="all">All partitions</option>
            {partitions.map(([id, label]) => (
              <option key={id} value={id}>
                {label} · 0x{id.toString(16).padStart(2, "0").toUpperCase()}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter by format"
            className="h-9 rounded-md border bg-card px-3 text-sm"
            value={p.format}
            onChange={(e) => p.setFormat(e.target.value)}
          >
            <option value="all">All formats</option>
            {formats.map((f) => (
              <option key={f} value={f}>
                {f.replace("_", " ")}
              </option>
            ))}
          </select>
          <div className="flex rounded-md border p-0.5">
            <Button
              size="icon"
              variant={p.view === "table" ? "secondary" : "ghost"}
              onClick={() => p.setView("table")}
              aria-label="Table view"
              aria-pressed={p.view === "table"}
              className="h-8 w-8"
            >
              <List size={16} />
            </Button>
            <Button
              size="icon"
              variant={p.view === "grid" ? "secondary" : "ghost"}
              onClick={() => p.setView("grid")}
              aria-label="Card view"
              aria-pressed={p.view === "grid"}
              className="h-8 w-8"
            >
              <Grid2X2 size={16} />
            </Button>
          </div>
        </div>
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        Text search and filters cover {p.records.length} loaded records
        {p.hasMore ? "; more records are available" : ""}. Exact key lookup
        searches the database.
      </p>
    </div>
  );
}
