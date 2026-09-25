import {
  Activity,
  Binary,
  Database,
  HardDrive,
  ShieldCheck,
} from "lucide-react";
import { SessionUser, TelemetryStats } from "../types";
import { Button } from "./ui/button";

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
export function OverviewView({
  stats,
  statsError,
  recordsError,
  user,
  loaded,
  hasMore,
  updatedAt,
  onRetry,
  onAnalyze,
}: Props) {
  return (
    <section aria-labelledby="overview-title" className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
            Database
          </p>
          <h1
            id="overview-title"
            className="mt-1 text-3xl font-semibold tracking-tight"
          >
            Overview
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connection and engine details reported by BeastDB.
          </p>
        </div>
        <Button variant="outline" onClick={onAnalyze}>
          <Binary size={16} />
          Key analyzer
        </Button>
      </div>
      {statsError && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm"
        >
          <span>{statsError}</span>
          <Button variant="outline" size="sm" onClick={onRetry}>
            Retry
          </Button>
        </div>
      )}
      {recordsError && recordsError !== statsError && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm"
        >
          <span>{recordsError}</span>
          <Button variant="outline" size="sm" onClick={onRetry}>
            Retry
          </Button>
        </div>
      )}
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div className="rounded-lg border bg-card p-5 shadow-sm">
          <Activity size={18} className="text-muted-foreground" />
          <p className="mt-4 text-xs text-muted-foreground">Connection</p>
          <p className="mt-1 text-xl font-semibold">
            {stats ? "Connected" : "Unavailable"}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            {stats ? "Telemetry request succeeded" : "Telemetry request failed"}
          </p>
        </div>
        <div className="rounded-lg border bg-card p-5 shadow-sm">
          <Database size={18} className="text-muted-foreground" />
          <p className="mt-4 text-xs text-muted-foreground">Loaded records</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">
            {loaded.toLocaleString()}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            {recordsError
              ? "Record load needs attention"
              : hasMore
                ? "More pages available"
                : "All scanned pages loaded"}
          </p>
        </div>
        <div className="rounded-lg border bg-card p-5 shadow-sm">
          <HardDrive size={18} className="text-muted-foreground" />
          <p className="mt-4 text-xs text-muted-foreground">
            Write-ahead log sequence
          </p>
          <p className="mt-1 text-xl font-semibold tabular-nums">
            {stats?.lsn?.toLocaleString() ?? "—"}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            Reported by engine
          </p>
        </div>
        <div className="rounded-lg border bg-card p-5 shadow-sm">
          <ShieldCheck size={18} className="text-muted-foreground" />
          <p className="mt-4 text-xs text-muted-foreground">Engine role</p>
          <p className="mt-1 text-xl font-semibold">{stats?.role || "—"}</p>
          <p className="mt-2 text-xs text-muted-foreground">
            {stats?.version
              ? `Version ${stats.version}`
              : "Version unavailable"}
          </p>
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border bg-card p-6">
          <h2 className="font-semibold">Engine information</h2>
          <dl className="mt-4 divide-y text-sm">
            <div className="flex justify-between gap-4 py-3">
              <dt className="text-muted-foreground">Storage mode</dt>
              <dd>{stats?.mode || "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-3">
              <dt className="text-muted-foreground">Last record refresh</dt>
              <dd>{updatedAt ? updatedAt.toLocaleTimeString() : "—"}</dd>
            </div>
          </dl>
        </div>
        <div className="rounded-lg border bg-card p-6">
          <h2 className="font-semibold">Your access</h2>
          <dl className="mt-4 divide-y text-sm">
            <div className="flex justify-between gap-4 py-3">
              <dt className="text-muted-foreground">Signed in as</dt>
              <dd>{user?.username || "Unavailable"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-3">
              <dt className="text-muted-foreground">Permission</dt>
              <dd className="capitalize">{user?.role || "Unavailable"}</dd>
            </div>
          </dl>
          <p className="mt-4 text-xs text-muted-foreground">
            Only admins can create or delete records.
          </p>
        </div>
      </div>
    </section>
  );
}
