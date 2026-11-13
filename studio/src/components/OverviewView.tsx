"use client";
import { Binary } from "lucide-react";
import { SessionUser, TelemetryStats } from "../types";
import { formatBytes } from "../utils/format";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { BufferPoolMap, WalHead } from "./EnginePanel";
import { PartitionChart } from "./PartitionChart";
import { StatCard, StatGrid } from "./StatCard";
import { getRegisteredPartitions } from "../utils/key-decoder";

interface Props {
  stats: TelemetryStats | null;
  statsError: string | null;
  recordsError: string | null;
  user: SessionUser | null;
  loaded: number;
  hasMore: boolean;
  updatedAt: Date | null;
  onRetry: () => void;
  onAnalyze: () => void;
}

export function OverviewView({ stats, statsError, recordsError, user, loaded, hasMore, updatedAt, onRetry, onAnalyze }: Props) {
  const chartData = (stats?.partitionCounts
    ? Object.entries(stats.partitionCounts).map(([prefixStr, count]) => {
        const prefix = Number(prefixStr);
        const registered = getRegisteredPartitions().find((p) => p.prefix === prefix);
        return { prefix, count, label: registered?.label || `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}`, color: registered?.color || "emerald" };
      })
    : []
  ).filter((d) => d.count > 0);

  const totalRecords = chartData.reduce((n, d) => n + d.count, 0);
  const isHealthy = Boolean(stats);
  const dirtyPages = stats?.storage?.dirtyPages ?? 0;
  const poolSize = stats?.storage?.poolSize ?? 64;
  const cachedPages = stats?.storage?.cachedPages ?? 0;
  const isDirtyHeavy = dirtyPages > poolSize * 0.8;
  const statusLabel = !isHealthy ? "Offline" : isDirtyHeavy ? "High Dirty Load" : "Optimal (Verified)";
  const statusColor = !isHealthy ? "bg-destructive" : isDirtyHeavy ? "bg-warning" : "bg-primary";

  return (
    <section aria-labelledby="overview-title" className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 id="overview-title" className="text-[22px] font-bold leading-tight tracking-tight text-foreground">Overview</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {isHealthy ? "Engine telemetry and measured vitals." : "Telemetry is unavailable right now."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-flex h-9 items-center gap-2 rounded-lg border border-border bg-card px-3 text-sm">
            <span className={`h-2 w-2 rounded-full ${statusColor}`} />
            {statusLabel}
          </span>
          <Button variant="outline" size="sm" onClick={onAnalyze} className="h-9 gap-2 text-sm">
            <Binary size={15} /> Key Analyzer
          </Button>
        </div>
      </div>

      {(statsError || recordsError) && (
        <div role="alert" className="flex items-center justify-between rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
          <span>{statsError || recordsError}</span>
          <Button variant="outline" size="sm" onClick={onRetry} className="h-7 text-xs">Retry</Button>
        </div>
      )}

      <StatGrid>
        <StatCard label="Records" value={totalRecords.toLocaleString()} hint={`${chartData.length} partition${chartData.length === 1 ? "" : "s"}`} />
        <StatCard label="Log sequence number" value={(stats?.lsn ?? 0).toLocaleString()} hint="WAL head" />
        <StatCard label="Dirty pages" value={`${dirtyPages}`} hint={`of ${poolSize} in buffer pool`} tone={isDirtyHeavy ? "warning" : "muted"} />
        <StatCard label="Storage" value={stats?.storage ? formatBytes(stats.storage.diskSizeBytes) : "—"} hint={`${stats?.storage?.totalPages ?? 0} pages`} />
      </StatGrid>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="p-6 lg:col-span-2"><BufferPoolMap storage={stats?.storage} /></Card>
        <Card className="p-6"><WalHead lsn={stats?.lsn} storage={stats?.storage} /></Card>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Stat label="Node role" value={(stats?.role || "standalone").toUpperCase()} />
        <Stat label="Version" value={`v${stats?.version || "1.0.0"}`} />
      </div>

      {/* Partition Distribution */}
      {chartData.length > 0 && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-semibold">Keyspace Distribution by Partition</CardTitle>
              <Badge variant="mono">{chartData.length} Partitions Registered</Badge>
            </div>
          </CardHeader>
          <CardContent>
            <PartitionChart data={chartData} />
          </CardContent>
        </Card>
      )}

      {/* Runtime Parameters & Session Grid */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader><CardTitle className="text-sm font-semibold">Engine Runtime Parameters</CardTitle></CardHeader>
          <CardContent className="divide-y divide-border text-xs">
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Storage Hierarchy</span><span className="font-mono text-foreground">4KB Slotted Pages / Slotted Array</span></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Durability Model</span><span className="font-mono text-foreground">ARIES WAL + Append-Only tombstones</span></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Index Structure</span><span className="font-mono text-foreground">On-Disk B+ Tree (Fanout ~200)</span></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Last Telemetry Sync</span><span className="font-mono text-muted-foreground">{updatedAt ? updatedAt.toLocaleTimeString() : "—"}</span></div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-sm font-semibold">Current Session Privileges</CardTitle></CardHeader>
          <CardContent className="divide-y divide-border text-xs">
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Authenticated Subject</span><span className="font-mono font-medium text-foreground">{user?.username || "Anonymous"}</span></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Authorization Scope</span><Badge variant={user?.role === "admin" ? "lime" : "mono"}>{user?.role || "viewer"}</Badge></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Write Operations</span><span className={user?.role === "admin" ? "text-primary font-medium" : "text-muted-foreground"}>{user?.role === "admin" ? "Enabled (Full I/O)" : "Disabled (Read-Only)"}</span></div>
            <div className="flex justify-between py-2.5"><span className="text-muted-foreground">Session Mode</span><span className="font-mono text-muted-foreground">HMAC-Signed Cookie</span></div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card className="flex items-baseline justify-between gap-3 px-5 py-4">
      <span className="shrink-0 text-xs text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate font-mono text-lg font-semibold text-foreground" title={value}>{value}</span>
    </Card>
  );
}
