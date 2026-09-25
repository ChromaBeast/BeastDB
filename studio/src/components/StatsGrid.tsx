"use client";

import React from "react";
import { HardDrive, Cpu, ShieldCheck, Activity } from "lucide-react";
import { TelemetryStats } from "../types";

interface StatsGridProps {
  stats: TelemetryStats | null;
  recordCount: number;
}

export const StatsGrid: React.FC<StatsGridProps> = ({ stats, recordCount }) => {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {/* Total Records */}
      <div className="group relative overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5 backdrop-blur-xl transition-all duration-300 hover:border-purple-500/40 hover:shadow-lg hover:shadow-purple-500/10">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wider text-slate-400">Total Records</span>
          <div className="rounded-lg bg-purple-500/10 p-2 text-purple-400 ring-1 ring-purple-500/20">
            <HardDrive className="h-4 w-4" />
          </div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span className="text-2xl font-bold tracking-tight text-white font-mono">{recordCount}</span>
          <span className="text-xs text-emerald-400 font-medium">B+ Tree Indexed</span>
        </div>
        <div className="mt-2 text-xs text-slate-400">Sub-microsecond point reads</div>
      </div>

      {/* Buffer Pool Memory */}
      <div className="group relative overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5 backdrop-blur-xl transition-all duration-300 hover:border-cyan-500/40 hover:shadow-lg hover:shadow-cyan-500/10">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wider text-slate-400">Buffer Pool Cache</span>
          <div className="rounded-lg bg-cyan-500/10 p-2 text-cyan-400 ring-1 ring-cyan-500/20">
            <Cpu className="h-4 w-4" />
          </div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span className="text-2xl font-bold tracking-tight text-white font-mono">1,024</span>
          <span className="text-xs text-slate-400">4KB frames</span>
        </div>
        {/* Visual Memory Meter */}
        <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
          <div className="h-full w-[38%] rounded-full bg-gradient-to-r from-cyan-500 to-purple-500" />
        </div>
      </div>

      {/* Durability Guard */}
      <div className="group relative overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5 backdrop-blur-xl transition-all duration-300 hover:border-emerald-500/40 hover:shadow-lg hover:shadow-emerald-500/10">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wider text-slate-400">Crash Durability</span>
          <div className="rounded-lg bg-emerald-500/10 p-2 text-emerald-400 ring-1 ring-emerald-500/20">
            <ShieldCheck className="h-4 w-4" />
          </div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span className="text-2xl font-bold tracking-tight text-white font-mono">#{stats?.lsn ?? 0}</span>
          <span className="text-xs text-emerald-400 font-medium">WAL Active</span>
        </div>
        <div className="mt-2 text-xs text-slate-400">IEEE CRC32 torn-write protection</div>
      </div>

      {/* Engine Status */}
      <div className="group relative overflow-hidden rounded-2xl border border-white/[0.08] bg-white/[0.02] p-5 backdrop-blur-xl transition-all duration-300 hover:border-indigo-500/40 hover:shadow-lg hover:shadow-indigo-500/10">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wider text-slate-400">Engine Mode</span>
          <div className="rounded-lg bg-indigo-500/10 p-2 text-indigo-400 ring-1 ring-indigo-500/20">
            <Activity className="h-4 w-4" />
          </div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span className="text-2xl font-bold tracking-tight text-white">{stats?.role ?? "Leader"}</span>
          <span className="text-xs text-purple-400 font-mono">v{stats?.version ?? "0.1.0"}</span>
        </div>
        <div className="mt-2 text-xs text-slate-400">Streaming replication ready</div>
      </div>
    </div>
  );
};
