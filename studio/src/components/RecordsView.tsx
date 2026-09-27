"use client";
import { useEffect, useMemo, useRef, useState } from "react";
import { SearchResponse, UniversalRecord, ViewMode } from "../types";
import { Button } from "./ui/button";
import { Breadcrumb } from "./Breadcrumb";
import { GlobalSearchBar, SearchMode } from "./GlobalSearchBar";
import { SearchCoverageBanner } from "./SearchCoverageBanner";
import { RecordsToolbar } from "./RecordsToolbar";
import { ExplorerContent } from "./ExplorerContent";
import { buildPartitionList } from "../utils/key-decoder";
import { usePartitionMetadata } from "../hooks/usePartitionMetadata";

interface Props {
  records: UniversalRecord[];
  selected: UniversalRecord | null;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  canWrite: boolean;
  error: string | null;
  partitionCounts?: Record<string, number>;
  onRetry: () => void;
  onLoadMore: () => void;
  onLoadPartition?: (prefix: number) => Promise<void>;
  onSelect: (record: UniversalRecord | null) => void;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: (record: UniversalRecord) => void;
  onNew: () => void;
  onLookup: (key: string) => Promise<UniversalRecord>;
  onSearch?: (q: string, opts?: { prefix?: number; start?: string; maxScan?: number }) => Promise<SearchResponse>;
  onScanRange?: (startKey: string, endKey: string) => Promise<UniversalRecord[]>;
  onNotice: (message: string, error?: boolean) => void;
  onOpenTokens?: () => void;
}

export function RecordsView(p: Props) {
  const metadataRevision = usePartitionMetadata(p.partitionCounts);
  const initialized = useRef(false);
  const [pane, setPane] = useState<"partitions" | "documents" | "details">("documents");
  const [searchMode, setSearchMode] = useState<SearchMode>("text");
  const [viewMode, setViewMode] = useState<ViewMode>("table");
  const [statusFilter, setStatusFilter] = useState("all");
  const [busy, setBusy] = useState(false);
  const [isSearchMode, setIsSearchMode] = useState(false);
  const [searchResults, setSearchResults] = useState<UniversalRecord[] | null>(null);
  const [searchMeta, setSearchMeta] = useState<SearchResponse & { query: string } | null>(null);

  const partitions = useMemo(
    () => buildPartitionList(p.records, p.partitionCounts),
    [p.records, p.partitionCounts, metadataRevision]
  );

  const [partition, setPartition] = useState<string>("1");

  useEffect(() => {
    if (initialized.current || !p.partitionCounts || partitions.length === 0) return;
    initialized.current = true;
    const first = partitions.some((item) => item.prefix === 1) ? 1 : partitions[0].prefix;
    setPartition(String(first));
    void p.onLoadPartition?.(first);
  }, [partitions, p.partitionCounts, p.onLoadPartition]);

  const totalCount = useMemo(() => {
    if (p.partitionCounts) return Object.values(p.partitionCounts).reduce((acc, c) => acc + c, 0);
    return p.records.length;
  }, [p.partitionCounts, p.records.length]);

  const activeRecords = isSearchMode && searchResults ? searchResults : p.records;

  const filteredRecords = useMemo(() => {
    if (statusFilter === "all") return activeRecords;
    return activeRecords.filter((r) => String(r.status ?? "").trim().toLowerCase() === statusFilter);
  }, [activeRecords, statusFilter]);

  const choosePartition = (val: string) => {
    setPartition(val);
    setStatusFilter("all");
    setIsSearchMode(false);
    setSearchResults(null);
    p.onSelect(null);
    setPane("documents");
    void p.onLoadPartition?.(Number(val));
  };

  const withBusy = async (fn: () => Promise<void>) => {
    setBusy(true);
    try { await fn(); } catch (e) { p.onNotice((e as Error).message, true); } finally { setBusy(false); }
  };

  const handleSearchText = (query: string, prefix?: number) => withBusy(async () => {
    if (!p.onSearch) return;
    const res = await p.onSearch(query, { prefix });
    setSearchResults(res.records);
    setSearchMeta({ ...res, query });
    setIsSearchMode(true);
    p.onNotice(`Found ${res.count} matches across ${res.scannedCount} keys.`);
  });

  const handleContinueScan = () => withBusy(async () => {
    if (!p.onSearch || !searchMeta?.nextKey) return;
    const res = await p.onSearch(searchMeta.query, { prefix: searchMeta.prefix, start: searchMeta.nextKey });
    setSearchResults((curr) => [...(curr || []), ...res.records]);
    setSearchMeta({
      ...res, query: searchMeta.query,
      count: (searchMeta.count || 0) + res.count,
      scannedCount: (searchMeta.scannedCount || 0) + res.scannedCount,
    });
    p.onNotice(`Scanned +${res.scannedCount} keys. Total: ${searchMeta.count + res.count}`);
  });

  const handleLookupKey = (key: string) => withBusy(async () => {
    const rec = await p.onLookup(key);
    setPartition(String(rec.prefix));
    p.onSelect(rec);
    setPane("details");
  });

  const handleScanRange = (startKey: string, endKey: string) => withBusy(async () => {
    if (!p.onScanRange) return;
    const recs = await p.onScanRange(startKey, endKey);
    setSearchResults(recs);
    setIsSearchMode(true);
    setSearchMeta(null);
    p.onNotice(`Retrieved ${recs.length} records in range.`);
  });

  const partitionTitle = partitions.find((g) => String(g.prefix) === partition)?.prefixLabel || `Collection ${Number(partition).toString(16).padStart(2, "0").toUpperCase()}`;

  return (
    <section aria-labelledby="records-title" className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <Breadcrumb partitionLabel={partitionTitle} recordLabel={p.selected?.primaryLabel} onBack={() => setPane("partitions")} />
          <h1 id="records-title" className="mt-1 text-2xl font-bold tracking-tight">Explorer</h1>
        </div>
      </div>

      <GlobalSearchBar
        mode={searchMode} onModeChange={setSearchMode}
        partitions={partitions} activePartition={partition}
        onSearchText={handleSearchText} onLookupKey={handleLookupKey} onScanRange={handleScanRange}
        busy={busy}
      />

      {isSearchMode && searchMeta && (
        <SearchCoverageBanner
          query={searchMeta.query} count={searchMeta.count} scannedCount={searchMeta.scannedCount}
          hasMore={searchMeta.hasMore} prefix={searchMeta.prefix}
          prefixLabel={partitions.find((g) => g.prefix === searchMeta.prefix)?.prefixLabel}
          loadingMore={busy} onContinueScan={handleContinueScan}
          onClear={() => { setIsSearchMode(false); setSearchResults(null); setSearchMeta(null); }}
        />
      )}

      {p.error && (
        <div role="alert" className="flex items-center justify-between rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
          <span>{p.error}</span>
          <Button variant="outline" size="sm" onClick={p.onRetry}>Retry</Button>
        </div>
      )}

      {/* Toolbar with dynamic status filter pills & view switcher */}
      <RecordsToolbar
        records={activeRecords}
        activeStatus={statusFilter}
        onSelectStatus={setStatusFilter}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        canWrite={p.canWrite}
        onNew={p.onNew}
      />

      {/* Main Multi-Mode Explorer Content */}
      <ExplorerContent
        viewMode={viewMode} records={filteredRecords} selected={p.selected}
        loading={p.loading} loadingMore={p.loadingMore} hasMore={p.hasMore}
        partitionTitle={isSearchMode ? "Search Results" : partitionTitle}
        canWrite={p.canWrite} pane={pane} partitions={partitions} partition={partition}
        totalCount={totalCount} partitionCounts={p.partitionCounts}
        onSelect={p.onSelect} onSetPane={setPane} onLoadMore={p.onLoadMore} onNew={p.onNew}
        onChoosePartition={choosePartition} onDelete={p.onDelete} onDeleteUser={p.onDeleteUser}
        onEdit={p.onEdit ? () => p.onEdit!(p.selected!) : undefined}
        onNotice={p.onNotice} onScanRange={p.onScanRange}
      />
    </section>
  );
}
