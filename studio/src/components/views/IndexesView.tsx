"use client";
import { useCallback, useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/button";
import { StatCard, StatGrid } from "../StatCard";
import { formatBytes } from "../../utils/format";
import { PartitionTable, PartitionRow } from "../PartitionTable";
import { BufferPoolCard, PrimaryIndexCard, SecondaryIndexCard } from "../IndexCards";
import { api } from "../../lib/api-client";
import { getPartitionMeta } from "../../utils/key-decoder";
import type { PartitionConfig, TelemetryStats } from "../../types";
import type { ViewProps } from "./types";

export function IndexesView({ stats, onRefresh }: ViewProps) {
  const [counted, setCounted] = useState<TelemetryStats | null>(null);
  const [labels, setLabels] = useState<PartitionConfig[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    const [s, p] = await Promise.allSettled([
      api<TelemetryStats>("/api/stats?counts=true"),
      api<PartitionConfig[]>("/api/partitions"),
    ]);
    if (s.status === "fulfilled") setCounted(s.value);
    else setError(s.reason?.message ?? "Could not load partition counts.");
    if (p.status === "fulfilled") setLabels(p.value ?? []);
    setLoading(false);
  }, []);

  useEffect(() => { void load(); }, [load]);

  const storage = counted?.storage ?? stats?.storage;
  const counts = counted?.partitionCounts ?? stats?.partitionCounts;

  const rows: PartitionRow[] = useMemo(
    () =>
      Object.entries(counts ?? {})
        .map(([k, count]) => {
          const prefix = Number(k);
          const label = labels.find((l) => l.prefix === prefix)?.label ?? getPartitionMeta(prefix).label;
          return { prefix, label, count };
        })
        .filter((r) => r.count > 0)
        .sort((a, b) => b.count - a.count),
    [counts, labels],
  );

  return (
    <section aria-labelledby="indexes-title" className="space-y-6">
      <PageHeader
        id="indexes-title"
        title="Indexes"
        subtitle="How records are located: the primary B+ tree and the secondary text index."
        actions={
          <Button variant="outline" size="sm" onClick={() => { onRefresh(); void load(); }} disabled={loading}>
            <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} aria-hidden />Refresh
          </Button>
        }
      />

      {error && (
        <div role="alert" className="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <StatGrid cols={3}>
        <StatCard label="Indexes" value="2" hint="primary B+ tree · text index" />
        <StatCard label="Data file" value={storage ? formatBytes(storage.diskSizeBytes) : "—"} hint={`${storage?.totalPages ?? 0} pages × 4 KiB`} />
        <StatCard label="Partitions in use" value={`${rows.length}`} hint={`${rows.reduce((n, r) => n + r.count, 0).toLocaleString()} records`} />
      </StatGrid>

      <div className="grid gap-4 lg:grid-cols-3">
        <PrimaryIndexCard storage={storage} />
        <BufferPoolCard storage={storage} />
        <SecondaryIndexCard />
      </div>

      <PartitionTable rows={rows} />
    </section>
  );
}
