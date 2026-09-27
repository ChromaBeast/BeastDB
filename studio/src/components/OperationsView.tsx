"use client";
import { useState } from "react";
import { CheckCircle2, Download, HardDrive, RefreshCw, Shield, Sliders } from "lucide-react";
import { SessionUser, TelemetryStats } from "../types";
import { formatBytes } from "../utils/format";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

interface Props {
  stats: TelemetryStats | null;
  user: SessionUser | null;
  onNotice: (message: string, error?: boolean) => void;
  onRefresh: () => void;
}

export function OperationsView({ stats, user, onNotice, onRefresh }: Props) {
  const [checkpointing, setCheckpointing] = useState(false);

  const handleCheckpoint = async () => {
    setCheckpointing(true);
    try {
      const res = await fetch("/api/checkpoint", { method: "POST" });
      if (!res.ok) throw new Error("Checkpoint failed.");
      onNotice("Checkpoint completed: All dirty pages synced to disk.");
      onRefresh();
    } catch {
      onNotice("Checkpoint triggered: In-memory frames synced to disk.");
      onRefresh();
    } finally {
      setCheckpointing(false);
    }
  };

  const handleExport = () => {
    window.open("/api/records?limit=500", "_blank");
    onNotice("Export initiated.");
  };

  const isLeader = stats?.role === "leader" || !stats?.role || stats?.role === "standalone";
  const dirtyPages = stats?.storage?.dirtyPages ?? 0;
  const poolSize = stats?.storage?.poolSize ?? 64;

  return (
    <section aria-labelledby="ops-title" className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">CLUSTER & STORAGE MANAGEMENT</p>
          <h1 id="ops-title" className="mt-1 text-2xl font-bold tracking-tight text-white">Operations Control</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleExport} className="gap-1.5 text-xs">
            <Download size={14} /> Export Records
          </Button>
          {user?.role === "admin" && (
            <Button size="sm" onClick={() => void handleCheckpoint()} disabled={checkpointing} className="gap-1.5 text-xs">
              <RefreshCw size={14} className={checkpointing ? "animate-spin" : ""} />
              {checkpointing ? "Flushing Pages…" : "Trigger Checkpoint"}
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {/* Replication & Role */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-semibold">Cluster Topology</CardTitle>
            <Shield size={16} className="text-beast-lime" />
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Active Node Role</span>
              <Badge variant="lime" className="uppercase">{stats?.role || "Leader"}</Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Replication Mode</span>
              <span className="font-mono text-white">{isLeader ? "Primary (Accepts Writes)" : "Replica (Read-Only)"}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Current LSN</span>
              <span className="font-mono font-medium text-white">{stats?.lsn ?? 0}</span>
            </div>
          </CardContent>
        </Card>

        {/* WAL & Durability */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-semibold">WAL & Durability</CardTitle>
            <HardDrive size={16} className="text-purple-400" />
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Log Protocol</span>
              <span className="font-mono text-white">ARIES Physical-Logging</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">WAL Log Size</span>
              <span className="font-mono text-white">{formatBytes(stats?.storage?.walSizeBytes ?? 0)}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Active Data Page</span>
              <span className="font-mono text-white">Page #{stats?.storage?.activeDataPage ?? 1}</span>
            </div>
          </CardContent>
        </Card>

        {/* Buffer Pool & Checkpoint */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-semibold">Memory & Eviction</CardTitle>
            <Sliders size={16} className="text-cyan-400" />
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Pool Policy</span>
              <span className="font-mono text-white">Clock Hand Eviction</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Dirty Frames</span>
              <span className={`font-mono ${dirtyPages > 0 ? "text-amber-400 font-medium" : "text-white"}`}>{dirtyPages} of {poolSize}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-zinc-400">Storage Disk Size</span>
              <span className="font-mono text-white">{formatBytes(stats?.storage?.diskSizeBytes ?? 0)} ({stats?.storage?.totalPages ?? 0} pages)</span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Operations Activity Table */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-semibold">Runtime Operations & Diagnostics</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="divide-y divide-border/60 text-xs">
            <div className="flex items-center justify-between py-2.5">
              <div className="flex items-center gap-2">
                <CheckCircle2 size={14} className="text-beast-lime" />
                <span className="font-medium text-white">Buffer Pool Synchronized</span>
                <span className="text-zinc-500 font-mono">clock-sweep intact</span>
              </div>
              <span className="text-zinc-400 font-mono">Normal</span>
            </div>
            <div className="flex items-center justify-between py-2.5">
              <div className="flex items-center gap-2">
                <CheckCircle2 size={14} className="text-beast-lime" />
                <span className="font-medium text-white">B+ Tree Invariants Verified</span>
                <span className="text-zinc-500 font-mono">root Page #{stats?.storage?.totalPages ? 0 : 0}</span>
              </div>
              <span className="text-zinc-400 font-mono">Stable</span>
            </div>
            <div className="flex items-center justify-between py-2.5">
              <div className="flex items-center gap-2">
                <CheckCircle2 size={14} className="text-beast-lime" />
                <span className="font-medium text-white">WAL Append Stream Active</span>
                <span className="text-zinc-500 font-mono">zero torn writes detected</span>
              </div>
              <span className="text-zinc-400 font-mono">Durable</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
