"use client";

import React from "react";
import { Zap, Database, LogOut, RefreshCw, Binary } from "lucide-react";
import { TelemetryStats } from "../types";

interface HeaderProps {
  stats: TelemetryStats | null;
  onRefresh: () => void;
  onOpenKeyCalculator: () => void;
  isLoading: boolean;
}

export const Header: React.FC<HeaderProps> = ({
  stats,
  onRefresh,
  onOpenKeyCalculator,
  isLoading,
}) => {
  return (
    <header className="sticky top-0 z-40 w-full border-b border-white/[0.08] bg-[#090D16]/80 backdrop-blur-xl">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-3 sm:px-6">
        {/* Brand & Badges */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-purple-600/20 text-purple-400 ring-1 ring-purple-500/30">
              <Zap className="h-5 w-5 fill-purple-400/20" />
            </div>
            <div>
              <span className="text-base font-bold tracking-tight text-white sm:text-lg">
                BeastDB <span className="text-purple-400 font-medium text-xs sm:text-sm">Studio</span>
              </span>
            </div>
          </div>

          <div className="hidden items-center gap-2 sm:flex">
            <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-0.5 text-xs font-semibold text-emerald-400">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
              {stats?.status ?? "Healthy"}
            </span>
            <span className="inline-flex items-center gap-1 rounded-full border border-purple-500/20 bg-purple-500/10 px-2.5 py-0.5 text-xs font-semibold text-purple-300">
              <Database className="h-3 w-3" />
              {stats?.role ?? "Leader"}
            </span>
          </div>
        </div>

        {/* Actions & User */}
        <div className="flex items-center gap-2.5">
          <button
            onClick={onOpenKeyCalculator}
            className="flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 text-xs font-medium text-slate-300 transition-colors hover:bg-white/[0.08] hover:text-white"
            title="Inspect 64-bit key architecture"
          >
            <Binary className="h-3.5 w-3.5 text-purple-400" />
            <span className="hidden sm:inline">Key Analyzer</span>
          </button>

          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 text-xs font-medium text-slate-300 transition-colors hover:bg-white/[0.08] hover:text-white disabled:opacity-50"
            title="Refresh database records"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? "animate-spin" : ""}`} />
            <span className="hidden sm:inline">Refresh</span>
          </button>

          <div className="h-4 w-px bg-white/10" />

          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-gradient-to-tr from-purple-600 to-indigo-500 text-xs font-bold text-white shadow-inner">
              A
            </div>
            <div className="hidden flex-col sm:flex">
              <span className="text-xs font-medium text-white leading-none">admin</span>
              <span className="text-[10px] text-slate-400 font-mono">Superuser</span>
            </div>
          </div>

          <form action="/logout" method="POST" className="m-0">
            <button
              type="submit"
              className="flex items-center gap-1 rounded-lg border border-white/10 p-1.5 text-slate-400 transition hover:border-red-500/30 hover:bg-red-500/10 hover:text-red-400"
              title="Sign Out"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </form>
        </div>
      </div>
    </header>
  );
};
