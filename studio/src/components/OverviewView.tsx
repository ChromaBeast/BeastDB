"use client";
import { Activity, Binary, Database, HardDrive, RefreshCw, ShieldCheck } from "lucide-react";
import { SessionUser, TelemetryStats } from "../types";
import { formatBytes } from "../utils/format";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { PartitionChart } from "./PartitionChart";
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

  const isHealthy = Boolean(stats);
  const dirtyPages = stats?.storage?.dirtyPages ?? 0;
  const poolSize = stats?.storage?.poolSize ?? 64;
  const cachedPages = stats?.storage?.cachedPages ?? 0;
  const isDirtyHeavy = dirtyPages > poolSize * 0.8;
  const statusLabel = !isHealthy ? "Offline" : isDirtyHeavy ? "High Dirty Load" : "Optimal (Verified)";
  const statusColor = !isHealthy ? "bg-destructive" : isDirtyHeavy ? "bg-amber-400" : "bg-beast-lime";

  return (
    <section aria-labelledby="overview-title" className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">TELEMETRY & MEASURED VITALS</p>
          <h1 id="overview-title" className="mt-1 text-2xl font-bold tracking-tight text-white">System Overview</h1>
        </div>
        <Button variant="outline" size="sm" onClick={onAnalyze} className="gap-2 text-xs">
          <Binary size={15} /> Key Analyzer
        </Button>
      </div>

      {(statsError || recordsError) && (
        <div role="alert" className="flex items-center justify-between rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
          <span>{statsError || recordsError}</span>
          <Button variant="outline" size="sm" onClick={onRetry} className="h-7 text-xs">Retry</Button>
        </div>
      )}

      {/* Top 4 Metrics Grid with Measured Telemetry */}
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <span className="text-[11px] font-mono uppercase text-zinc-400">ENGINE STATE</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-beast-lime/10 text-beast-lime">
              <Activity size={15} />
            </div>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-2">
              <span className={`h-2.5 w-2.5 rounded-full ${statusColor} ${isHealthy ? "animate-pulse" : ""}`} />
              <div className="text-xl font-bold text-white">{statusLabel}</div>
            </div>
            <p className="mt-1 text-[11px] text-zinc-400 font-mono">
              {stats?.storage ? `${cachedPages}/${poolSize} frames cached` : "B+ Tree 4KB · ARIES WAL"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <span className="text-[11px] font-mono uppercase text-zinc-400">BUFFER POOL</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-cyan-400/10 text-cyan-400">
              <Database size={15} />
            </div>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold font-mono tracking-tight text-white">
              {stats?.storage ? `${cachedPages} / ${poolSize}` : `${loaded.toLocaleString()}`}
            </div>
            <p className="mt-1 text-[11px] text-zinc-400">
              {stats?.storage ? `${dirtyPages} dirty · ${stats.storage.pinnedPages} pinned` : "Active memory frames"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <span className="text-[11px] font-mono uppercase text-zinc-400">STORAGE & WAL</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-purple-400/10 text-purple-400">
              <HardDrive size={15} />
            </div>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold font-mono tracking-tight text-white">
              LSN {stats?.lsn?.toLocaleString() ?? "—"}
            </div>
            <p className="mt-1 text-[11px] text-zinc-400 font-mono">
              {stats?.storage ? `${formatBytes(stats.storage.diskSizeBytes)} disk · ${formatBytes(stats.storage.walSizeBytes)} WAL` : "Log sequence number"}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <span className="text-[11px] font-mono uppercase text-zinc-400">NODE TOPOLOGY</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-md bg-beast-lime/10 text-beast-lime">
              <ShieldCheck size={15} />
            </div>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-2">
              <Badge variant="lime" className="uppercase">{stats?.role || "Standalone"}</Badge>
            </div>
            <p className="mt-1.5 text-[11px] text-zinc-400 font-mono">v{stats?.version || "1.0.0"} · Active Pg: #{stats?.storage?.activeDataPage ?? 1}</p>
          </CardContent>
        </Card>
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
          <CardContent className="divide-y divide-zinc-800 text-xs">
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Storage Hierarchy</span><span className="font-mono text-white">4KB Slotted Pages / Slotted Array</span></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Durability Model</span><span className="font-mono text-white">ARIES WAL + Append-Only tombstones</span></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Index Structure</span><span className="font-mono text-white">On-Disk B+ Tree (Fanout ~200)</span></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Last Telemetry Sync</span><span className="font-mono text-zinc-400">{updatedAt ? updatedAt.toLocaleTimeString() : "—"}</span></div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-sm font-semibold">Current Session Privileges</CardTitle></CardHeader>
          <CardContent className="divide-y divide-zinc-800 text-xs">
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Authenticated Subject</span><span className="font-mono font-medium text-white">{user?.username || "Anonymous"}</span></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Authorization Scope</span><Badge variant={user?.role === "admin" ? "lime" : "mono"}>{user?.role || "viewer"}</Badge></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Write Operations</span><span className={user?.role === "admin" ? "text-beast-lime font-medium" : "text-zinc-500"}>{user?.role === "admin" ? "Enabled (Full I/O)" : "Disabled (Read-Only)"}</span></div>
            <div className="flex justify-between py-2.5"><span className="text-zinc-400">Session Mode</span><span className="font-mono text-zinc-400">HMAC-Signed Cookie</span></div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
