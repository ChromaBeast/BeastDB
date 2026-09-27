"use client";
import { useEffect, useMemo, useState } from "react";
import { FileText } from "lucide-react";
import { SearchResponse, UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { PartitionRail } from "./PartitionRail";
import { DocumentList } from "./DocumentList";
import { Breadcrumb } from "./Breadcrumb";
import { RecordDetails } from "./RecordDetails";
import { GlobalSearchBar, SearchMode } from "./GlobalSearchBar";
import { SearchCoverageBanner } from "./SearchCoverageBanner";
import { buildPartitionList } from "../utils/key-decoder";

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

type Pane = "partitions" | "documents" | "details";

export function RecordsView(p: Props) {
  const [pane, setPane] = useState<Pane>("documents");
  const [searchMode, setSearchMode] = useState<SearchMode>("text");
  const [busy, setBusy] = useState(false);
  const [isSearchMode, setIsSearchMode] = useState(false);
  const [searchResults, setSearchResults] = useState<UniversalRecord[] | null>(null);
  const [searchMeta, setSearchMeta] = useState<SearchResponse & { query: string } | null>(null);

  const partitions = useMemo(
    () => buildPartitionList(p.records, p.partitionCounts),
    [p.records, p.partitionCounts]
  );

  const [partition, setPartition] = useState<string>("1");

  useEffect(() => {
    if (partitions.length > 0 && partition === "1" && !partitions.some(g => String(g.prefix) === "1")) {
      setPartition(String(partitions[0].prefix));
    }
  }, [partitions, partition]);

  const totalCount = useMemo(() => {
    if (p.partitionCounts) return Object.values(p.partitionCounts).reduce((acc, c) => acc + c, 0);
    return p.records.length;
  }, [p.partitionCounts, p.records.length]);

  const activeRecords = isSearchMode && searchResults ? searchResults : p.records;

  const choosePartition = (val: string) => {
    setPartition(val);
    setIsSearchMode(false);
    setSearchResults(null);
    p.onSelect(null);
    setPane("documents");
    if (p.onLoadPartition) {
      void p.onLoadPartition(Number(val));
    }
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
      ...res,
      query: searchMeta.query,
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

  const partitionTitle = partitions.find((g) => String(g.prefix) === partition)?.prefixLabel || `Partition 0x${Number(partition).toString(16).padStart(2, "0").toUpperCase()}`;

  return (
    <section aria-labelledby="records-title" className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <Breadcrumb partitionLabel={partitionTitle} recordLabel={p.selected?.primaryLabel} onBack={() => setPane("partitions")} />
          <h1 id="records-title" className="mt-1 text-2xl font-bold tracking-tight">Explorer</h1>
        </div>
      </div>

      <GlobalSearchBar
        mode={searchMode}
        onModeChange={setSearchMode}
        partitions={partitions}
        activePartition={partition}
        onSearchText={handleSearchText}
        onLookupKey={handleLookupKey}
        onScanRange={handleScanRange}
        busy={busy}
      />

      {isSearchMode && searchMeta && (
        <SearchCoverageBanner
          query={searchMeta.query}
          count={searchMeta.count}
          scannedCount={searchMeta.scannedCount}
          hasMore={searchMeta.hasMore}
          prefix={searchMeta.prefix}
          prefixLabel={partitions.find(g => g.prefix === searchMeta.prefix)?.prefixLabel}
          loadingMore={busy}
          onContinueScan={handleContinueScan}
          onClear={() => { setIsSearchMode(false); setSearchResults(null); setSearchMeta(null); }}
        />
      )}

      {p.error && (
        <div role="alert" className="flex items-center justify-between rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
          <span>{p.error}</span>
          <Button variant="outline" size="sm" onClick={p.onRetry}>Retry</Button>
        </div>
      )}

      <div className="grid min-h-0 flex-1 overflow-hidden rounded-xl border bg-card shadow-sm lg:grid-cols-[220px_minmax(320px,1fr)_minmax(380px,1.4fr)]">
        <div className={`${pane !== "partitions" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          <PartitionRail partitions={partitions} selected={partition} total={totalCount} hasMore={p.hasMore} hasFullCounts={Boolean(p.partitionCounts)} onSelect={choosePartition} />
        </div>
        <div className={`${pane !== "documents" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          <DocumentList records={activeRecords} selected={p.selected} loading={p.loading} loadingMore={p.loadingMore} hasMore={p.hasMore} title={isSearchMode ? "Search Results" : partitionTitle} shownCount={activeRecords.length} onLoadMore={p.onLoadMore} onSelect={(r) => { p.onSelect(r); setPane("details"); }} onBack={() => setPane("partitions")} canWrite={p.canWrite} onNew={p.onNew} />
        </div>
        <div className={`${pane !== "details" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          {p.selected ? (
            <RecordDetails record={p.selected} canWrite={p.canWrite} onDelete={p.onDelete} onDeleteUser={p.onDeleteUser} onEdit={p.onEdit ? () => p.onEdit!(p.selected!) : undefined} onNotice={p.onNotice} onScanRange={p.onScanRange} onSelect={(r) => { p.onSelect(r); setPane("details"); }} partitionCounts={p.partitionCounts} />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
              <div className="rounded-xl border bg-muted p-4"><FileText size={24} className="text-muted-foreground" /></div>
              <h2 className="mt-4 font-medium text-foreground">Select a record</h2>
              <p className="mt-1 max-w-xs text-sm text-muted-foreground">Choose a record from the list to view its fields, raw payload, and key breakdown.</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
