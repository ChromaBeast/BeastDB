"use client";
import { Columns2, LayoutGrid, Plus, Table as TableIcon } from "lucide-react";
import { UniversalRecord, ViewMode } from "../types";
import { StatusFilterBar } from "./StatusFilterBar";
import { Button } from "./ui/button";

interface Props {
  records: UniversalRecord[];
  activeStatus: string;
  onSelectStatus: (status: string) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  canWrite: boolean;
  onNew: () => void;
}

export function RecordsToolbar({
  records, activeStatus, onSelectStatus, viewMode, onViewModeChange, canWrite, onNew,
}: Props) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      {/* Dynamic Status Filter Pills */}
      <StatusFilterBar
        records={records}
        activeStatus={activeStatus}
        onSelectStatus={onSelectStatus}
      />

      {/* View Switcher + New Record Button */}
      <div className="flex items-center gap-2 ml-auto">
        <div className="flex items-center rounded-lg border border-border bg-muted/60 p-0.5 text-xs">
          <button
            type="button"
            onClick={() => onViewModeChange("table")}
            title="Spreadsheet Table View"
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 font-medium transition ${
              viewMode === "table"
                ? "bg-card text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <TableIcon size={13} />
            <span className="hidden sm:inline">Table</span>
          </button>

          <button
            type="button"
            onClick={() => onViewModeChange("grid")}
            title="Poster / Card Gallery View"
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 font-medium transition ${
              viewMode === "grid"
                ? "bg-card text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <LayoutGrid size={13} />
            <span className="hidden sm:inline">Gallery</span>
          </button>

          <button
            type="button"
            onClick={() => onViewModeChange("split")}
            title="Classic 3-Pane Split View"
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 font-medium transition ${
              viewMode === "split"
                ? "bg-card text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            <Columns2 size={13} />
            <span className="hidden sm:inline">Split</span>
          </button>
        </div>

        {canWrite && (
          <Button size="sm" onClick={onNew} className="h-7 gap-1 text-xs shadow-sm">
            <Plus size={13} /> New Record
          </Button>
        )}
      </div>
    </div>
  );
}
