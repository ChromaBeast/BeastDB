"use client";
import { FileText, X } from "lucide-react";
import { UniversalRecord, ViewMode } from "../types";
import { PartitionRail } from "./PartitionRail";
import { DocumentList } from "./DocumentList";
import { RecordTableView } from "./RecordTableView";
import { RecordGridView } from "./RecordGridView";
import { RecordDetails } from "./RecordDetails";
import { Button } from "./ui/button";

interface Props {
  viewMode: ViewMode;
  records: UniversalRecord[];
  selected: UniversalRecord | null;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  partitionTitle: string;
  canWrite: boolean;
  pane: "partitions" | "documents" | "details";
  partitions: Array<{ prefix: number; count: number; prefixLabel: string }>;
  partition: string;
  totalCount: number;
  partitionCounts?: Record<string, number>;
  onSelect: (record: UniversalRecord | null) => void;
  onSetPane: (pane: "partitions" | "documents" | "details") => void;
  onLoadMore: () => void;
  onNew: () => void;
  onChoosePartition: (val: string) => void;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: (record: UniversalRecord) => void;
  onNotice: (message: string, error?: boolean) => void;
  onScanRange?: (startKey: string, endKey: string) => Promise<UniversalRecord[]>;
}

export function ExplorerContent(p: Props) {
  const isSplit = p.viewMode === "split";

  return (
    <div className={`grid min-h-0 flex-1 overflow-hidden rounded-xl border bg-card shadow-sm ${
      isSplit
        ? "lg:grid-cols-[220px_minmax(320px,1fr)_minmax(380px,1.4fr)]"
        : p.selected
        ? "lg:grid-cols-[220px_minmax(0,1fr)_420px]"
        : "lg:grid-cols-[220px_minmax(0,1fr)]"
    }`}>
      {/* Left Partition Rail */}
      <div className={`${p.pane !== "partitions" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
        <PartitionRail
          partitions={p.partitions}
          selected={p.partition}
          total={p.totalCount}
          hasMore={p.hasMore}
          hasFullCounts={Boolean(p.partitionCounts)}
          onSelect={p.onChoosePartition}
        />
      </div>

      {/* Main Content Area */}
      <div className={`${p.pane !== "documents" ? "hidden lg:flex" : "flex"} min-h-0 flex-col overflow-hidden`}>
        {isSplit ? (
          <DocumentList
            records={p.records}
            selected={p.selected}
            loading={p.loading}
            loadingMore={p.loadingMore}
            hasMore={p.hasMore}
            title={p.partitionTitle}
            shownCount={p.records.length}
            onLoadMore={p.onLoadMore}
            onSelect={(r) => { p.onSelect(r); p.onSetPane("details"); }}
            onBack={() => p.onSetPane("partitions")}
            canWrite={p.canWrite}
            onNew={p.onNew}
          />
        ) : p.viewMode === "table" ? (
          <RecordTableView
            records={p.records}
            selectedKey={p.selected?.keyStr}
            hasMore={p.hasMore}
            loadingMore={p.loadingMore}
            onLoadMore={p.onLoadMore}
            onSelect={(r) => { p.onSelect(r); p.onSetPane("details"); }}
          />
        ) : (
          <RecordGridView
            records={p.records}
            selectedKey={p.selected?.keyStr}
            hasMore={p.hasMore}
            loadingMore={p.loadingMore}
            onLoadMore={p.onLoadMore}
            onSelect={(r) => { p.onSelect(r); p.onSetPane("details"); }}
          />
        )}
      </div>

      {/* Details Side Panel */}
      {(isSplit || p.selected) && (
        <div className={`${p.pane !== "details" ? "hidden lg:flex" : "flex"} min-h-0 flex-col border-l border-border bg-card/60 relative`}>
          {!isSplit && p.selected && (
            <div className="absolute top-3 right-3 z-10">
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 rounded-full text-muted-foreground hover:text-foreground"
                onClick={() => p.onSelect(null)}
                aria-label="Close details"
              >
                <X size={15} />
              </Button>
            </div>
          )}

          {p.selected ? (
            <RecordDetails
              record={p.selected}
              canWrite={p.canWrite}
              onDelete={p.onDelete}
              onDeleteUser={p.onDeleteUser}
              onEdit={p.onEdit ? () => p.onEdit!(p.selected!) : undefined}
              onNotice={p.onNotice}
              onScanRange={p.onScanRange}
              onSelect={(r) => { p.onSelect(r); p.onSetPane("details"); }}
              partitionCounts={p.partitionCounts}
            />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
              <div className="rounded-xl border bg-muted p-4"><FileText size={24} className="text-muted-foreground" /></div>
              <h2 className="mt-4 font-medium text-foreground">Select a record</h2>
              <p className="mt-1 max-w-xs text-sm text-muted-foreground">Choose a record to inspect its fields and storage details.</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
