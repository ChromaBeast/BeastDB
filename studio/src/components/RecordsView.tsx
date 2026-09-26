"use client";
import { useMemo, useState } from "react";
import { Database, FileText, Plus, X } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { PartitionRail } from "./PartitionRail";
import { DocumentList } from "./DocumentList";
import { Breadcrumb } from "./Breadcrumb";
import { RecordDetails } from "./RecordDetails";

interface Props {
  records: UniversalRecord[];
  selected: UniversalRecord | null;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  canWrite: boolean;
  error: string | null;
  onRetry: () => void;
  onLoadMore: () => void;
  onSelect: (record: UniversalRecord | null) => void;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: (record: UniversalRecord) => void;
  onNew: () => void;
  onLookup: (key: string) => Promise<UniversalRecord>;
  onSearch?: (q: string, prefix?: number) => Promise<UniversalRecord[]>;
  onNotice: (message: string, error?: boolean) => void;
}

type Pane = "partitions" | "documents" | "details";

export function RecordsView(p: Props) {
  const [partition, setPartition] = useState("all");
  const [pane, setPane] = useState<Pane>("partitions");
  const [search, setSearch] = useState("");
  const [format, setFormat] = useState("all");
  const [looking, setLooking] = useState(false);
  const [isSearchMode, setIsSearchMode] = useState(false);
  const [searchResults, setSearchResults] = useState<UniversalRecord[] | null>(null);

  const partitions = useMemo(() => {
    const groups = new Map<number, { prefixLabel: string; count: number }>();
    for (const r of p.records) {
      const cur = groups.get(r.prefix);
      groups.set(r.prefix, { prefixLabel: r.prefixLabel, count: (cur?.count || 0) + 1 });
    }
    return [...groups.entries()]
      .sort(([a], [b]) => a - b)
      .map(([prefix, g]) => ({ prefix, count: g.count, prefixLabel: g.prefixLabel }));
  }, [p.records]);

  const formats = useMemo(() => [...new Set(p.records.map((r) => r.format))], [p.records]);

  const activeRecords = isSearchMode && searchResults ? searchResults : p.records;

  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    return activeRecords.filter(
      (r) =>
        (partition === "all" || String(r.prefix) === partition) &&
        (format === "all" || r.format === format) &&
        (!q || isSearchMode || [r.keyStr, r.primaryLabel, r.secondaryLabel || "", r.raw].some((v) => v.toLowerCase().includes(q)))
    );
  }, [activeRecords, partition, format, search, isSearchMode]);

  const handleLookup = async () => {
    const key = search.trim();
    if (!/^(0|[1-9]\d*)$/.test(key) || BigInt(key) > 18446744073709551615n) {
      p.onNotice("Enter a decimal key from 0 to 18446744073709551615.", true);
      return;
    }
    setLooking(true);
    try {
      const rec = await p.onLookup(key);
      setPartition(String(rec.prefix));
      p.onSelect(rec);
      setPane("details");
    } catch (e) {
      p.onNotice((e as Error).message, true);
    } finally {
      setLooking(false);
    }
  };

  const handleServerSearch = async () => {
    if (!p.onSearch || !search.trim()) return;
    setLooking(true);
    try {
      const pfx = partition !== "all" ? Number(partition) : undefined;
      const res = await p.onSearch(search.trim(), pfx);
      setSearchResults(res);
      setIsSearchMode(true);
      p.onNotice(`Found ${res.length} matches in database.`);
    } catch (e) {
      p.onNotice((e as Error).message, true);
    } finally {
      setLooking(false);
    }
  };

  const partitionTitle =
    partition === "all" ? "All records" : partitions.find((g) => String(g.prefix) === partition)?.prefixLabel || `Partition 0x${Number(partition).toString(16).padStart(2, "0").toUpperCase()}`;

  return (
    <section aria-labelledby="records-title" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <Breadcrumb partitionLabel={partitionTitle} recordLabel={p.selected?.primaryLabel} onBack={() => { setPartition("all"); setPane("partitions"); }} />
          <h1 id="records-title" className="mt-1 text-2xl font-bold tracking-tight">Records Explorer</h1>
        </div>
        <div className="flex items-center gap-2">
          {isSearchMode && (
            <Button variant="outline" size="sm" onClick={() => { setIsSearchMode(false); setSearchResults(null); }}>
              <X size={14} /> Clear Search ({searchResults?.length})
            </Button>
          )}
          {p.onSearch && search.trim() && !isSearchMode && (
            <Button variant="secondary" size="sm" disabled={looking} onClick={() => void handleServerSearch()}>
              Search DB for &ldquo;{search.slice(0, 15)}&rdquo;
            </Button>
          )}
          {p.canWrite && (
            <Button onClick={p.onNew} size="sm">
              <Plus size={16} /> New record
            </Button>
          )}
        </div>
      </div>
      {p.error && (
        <div role="alert" className="flex items-center justify-between rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
          <span>{p.error}</span>
          <Button variant="outline" size="sm" onClick={p.onRetry}>Retry</Button>
        </div>
      )}
      <div className="grid min-h-[660px] overflow-hidden rounded-xl border bg-card shadow-sm lg:grid-cols-[220px_minmax(280px,0.9fr)_minmax(380px,1.4fr)]">
        <div className={`${pane !== "partitions" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          <PartitionRail partitions={partitions} selected={partition} total={p.records.length} hasMore={p.hasMore} onSelect={(val) => { setPartition(val); p.onSelect(null); setPane("documents"); }} />
        </div>
        <div className={`${pane !== "documents" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          <DocumentList records={shown} selected={p.selected} loading={p.loading} loadingMore={p.loadingMore} hasMore={p.hasMore} search={search} format={format} formats={formats} looking={looking} title={partitionTitle} shownCount={shown.length} onSearchChange={setSearch} onFormatChange={setFormat} onExactLookup={() => void handleLookup()} onLoadMore={p.onLoadMore} onSelect={(r) => { p.onSelect(r); setPane("details"); }} onBack={() => setPane("partitions")} canWrite={p.canWrite} onNew={p.onNew} />
        </div>
        <div className={`${pane !== "details" ? "hidden lg:flex" : "flex"} min-h-0 flex-col`}>
          {p.selected ? (
            <RecordDetails record={p.selected} canWrite={p.canWrite} onDelete={p.onDelete} onDeleteUser={p.onDeleteUser} onEdit={p.onEdit ? () => p.onEdit!(p.selected!) : undefined} onNotice={p.onNotice} />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
              <div className="rounded-xl border bg-muted p-4"><FileText size={24} className="text-muted-foreground" /></div>
              <h2 className="mt-4 font-medium">Select a record</h2>
              <p className="mt-1 max-w-xs text-sm text-muted-foreground">Choose a record from the list to view its fields, key breakdown, and actions.</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
