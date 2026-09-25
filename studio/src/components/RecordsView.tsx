"use client";
import { useMemo, useState } from "react";
import { Database, Plus } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { RecordsToolbar } from "./RecordsToolbar";
import { RecordsResults } from "./RecordsResults";

interface Props {
  records: UniversalRecord[];
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  canWrite: boolean;
  error: string | null;
  onRetry: () => void;
  onLoadMore: () => void;
  onSelect: (record: UniversalRecord) => void;
  onNew: () => void;
  onLookup: (key: string) => Promise<UniversalRecord>;
  onNotice: (message: string, error?: boolean) => void;
}
export function RecordsView(p: Props) {
  const [search, setSearch] = useState("");
  const [prefix, setPrefix] = useState("all");
  const [format, setFormat] = useState("all");
  const [view, setView] = useState<"table" | "grid">("table");
  const [looking, setLooking] = useState(false);
  const shown = useMemo(
    () =>
      p.records.filter((r) => {
        if (prefix !== "all" && String(r.prefix) !== prefix) return false;
        if (format !== "all" && r.format !== format) return false;
        const q = search.trim().toLowerCase();
        return (
          !q ||
          [r.keyStr, r.primaryLabel, r.secondaryLabel || "", r.raw].some((s) =>
            s.toLowerCase().includes(q),
          )
        );
      }),
    [p.records, prefix, format, search],
  );
  const lookup = async () => {
    const key = search.trim();
    if (!/^(0|[1-9]\d*)$/.test(key) || BigInt(key) > 18446744073709551615n) {
      p.onNotice(
        "Enter a decimal key from 0 to 18446744073709551615 for exact lookup.",
        true,
      );
      return;
    }
    setLooking(true);
    try {
      p.onSelect(await p.onLookup(key));
    } catch (e) {
      p.onNotice((e as Error).message, true);
    } finally {
      setLooking(false);
    }
  };
  return (
    <section aria-labelledby="records-title" className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
            Data explorer
          </p>
          <h1
            id="records-title"
            className="mt-1 text-3xl font-semibold tracking-tight"
          >
            Records
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Browse values, inspect fields, and look up exact keys.
          </p>
        </div>
        {p.canWrite && (
          <Button onClick={p.onNew}>
            <Plus size={16} />
            New record
          </Button>
        )}
      </div>
      <RecordsToolbar
        records={p.records}
        hasMore={p.hasMore}
        search={search}
        setSearch={setSearch}
        prefix={prefix}
        setPrefix={setPrefix}
        format={format}
        setFormat={setFormat}
        view={view}
        setView={setView}
        looking={looking}
        onLookup={() => void lookup()}
      />
      {p.error && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm"
        >
          <span>{p.error}</span>
          <Button variant="outline" size="sm" onClick={p.onRetry}>
            Retry
          </Button>
        </div>
      )}
      {p.loading && !p.records.length ? (
        <div
          role="status"
          className="rounded-lg border bg-card p-12 text-center text-sm text-muted-foreground"
        >
          Loading records…
        </div>
      ) : p.error && !p.records.length ? null : shown.length === 0 ? (
        <div className="rounded-lg border bg-card p-12 text-center">
          <Database size={24} className="mx-auto text-muted-foreground" />
          <h2 className="mt-3 font-medium">
            {p.records.length ? "No matching records" : "No records yet"}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {p.records.length
              ? "Adjust the search or filters."
              : "Add a record to get started."}
          </p>
        </div>
      ) : (
        <RecordsResults
          records={shown}
          view={view}
          onSelect={p.onSelect}
          onNotice={p.onNotice}
        />
      )}
      <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
        <span>
          Showing {shown.length} of {p.records.length} loaded records
          {p.hasMore ? " · more available" : ""}
        </span>
        {p.hasMore && (
          <Button
            variant="outline"
            disabled={p.loadingMore}
            onClick={p.onLoadMore}
          >
            {p.loadingMore ? "Loading…" : "Load more records"}
          </Button>
        )}
      </div>
    </section>
  );
}
