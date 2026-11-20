"use client";
import { useState } from "react";
import { Activity, Lock, WifiOff } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Card } from "../ui/card";
import { MetricTile } from "../metrics/MetricTile";
import { RawMetrics } from "../metrics/RawMetrics";
import { Sparkline } from "../metrics/Sparkline";
import { MAX_POINTS, POLL_MS, useMetricsHistory, type MetricsPoint, type SeriesKey } from "../../hooks/useMetricsHistory";
import { useServerHistory } from "../../hooks/useServerHistory";
import { gauge, type Snapshot } from "../../utils/prometheus";
import { formatBytes } from "../../utils/format";
import type { ViewProps } from "./types";

const NA = "—";

function fmtSeconds(s: number | undefined): string {
  if (s === undefined) return NA;
  if (s < 1) return `${(s * 1000).toFixed(s < 0.01 ? 2 : 1)} ms`;
  return `${s.toFixed(2)} s`;
}
const fmtRate = (v: number | undefined) => (v === undefined ? NA : v >= 100 ? v.toFixed(0) : v.toFixed(2));
const fmtPct = (v: number | undefined) => (v === undefined ? NA : `${(v * 100).toFixed(v > 0 && v < 0.001 ? 3 : 1)}%`);
const fmtInt = (v: number | undefined) => (v === undefined ? NA : Math.round(v).toLocaleString());

function latest(history: MetricsPoint[], key: SeriesKey): number | undefined {
  for (let i = history.length - 1; i >= 0; i--) if (history[i][key] !== undefined) return history[i][key];
  return undefined;
}
const has = (history: MetricsPoint[], key: SeriesKey) => history.some((p) => p[key] !== undefined);
const col = (history: MetricsPoint[], key: SeriesKey) => history.map((p) => p[key]);

function Notice({ icon, title, body }: { icon: React.ReactNode; title: string; body: string }) {
  return (
    <Card className="flex flex-col items-center gap-2 px-6 py-14 text-center" role="status">
      <div className="text-muted-foreground">{icon}</div>
      <p className="text-sm font-semibold">{title}</p>
      <p className="max-w-md text-xs text-muted-foreground">{body}</p>
    </Card>
  );
}

function ChartCard({ title, value, history, k, tone, zeroBased }: { title: string; value: string; history: MetricsPoint[]; k: SeriesKey; tone?: string; zeroBased?: boolean }) {
  return (
    <SeriesCard
      title={title}
      value={value}
      values={col(history, k)}
      tone={tone}
      zeroBased={zeroBased}
      start={`-${Math.round((MAX_POINTS * POLL_MS) / 60000)} min`}
    />
  );
}

function SeriesCard({ title, value, values, tone, zeroBased, start }: { title: string; value: string; values: (number | undefined)[]; tone?: string; zeroBased?: boolean; start: string }) {
  return (
    <Card className="p-5">
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold">{title}</h2>
        <span className="font-mono text-xs text-muted-foreground">{value}</span>
      </div>
      <Sparkline values={values} label={title} tone={tone} height={96} zeroBased={zeroBased} className="mt-3" />
      <p className="mt-2 flex justify-between font-mono text-[10px] text-muted-foreground">
        <span>{start}</span>
        <span>now</span>
      </p>
    </Card>
  );
}

const RANGES = [
  { id: "live", label: "Live (5 min)", seconds: 0 },
  { id: "1h", label: "Last hour", seconds: 3600 },
  { id: "24h", label: "Last 24 hours", seconds: 86400 },
] as const;
type RangeId = (typeof RANGES)[number]["id"];

/** Charts from the server's own in-memory history: they survive page reloads but not a server restart. */
function HistoryCharts({ seconds }: { seconds: number }) {
  const { data, error } = useServerHistory(seconds, true);
  const pts = data?.points ?? [];
  const last = pts[pts.length - 1];
  const reach = pts.length > 1 ? Math.max(1, Math.round((pts[pts.length - 1].t - pts[0].t) / 60)) : 0;
  const start = reach > 0 && reach * 60 < seconds ? `-${reach} min (since server start)` : seconds >= 86400 ? "-24 h" : "-1 h";
  return (
    <>
      {error && <p role="alert" className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">Could not load history ({error}).</p>}
      <p className="text-xs text-muted-foreground">
        Sampled by the server every {data?.intervalSeconds ?? 10} s and kept in memory only, so it starts empty after a restart.
      </p>
      <div className="grid gap-4 lg:grid-cols-2">
        <SeriesCard title="Requests / s" value={last ? fmtRate(last.reqPerSec) : NA} values={pts.map((p) => p.reqPerSec)} start={start} />
        <SeriesCard title="WAL size" value={last ? formatBytes(last.walBytes) : NA} values={pts.map((p) => p.walBytes)} tone="text-warning" start={start} />
        <SeriesCard title="Dirty pages" value={last ? fmtInt(last.dirtyPages) : NA} values={pts.map((p) => p.dirtyPages)} tone="text-info" start={start} />
        <SeriesCard title="Replica lag (max, LSNs)" value={last ? fmtInt(last.replicaLag) : NA} values={pts.map((p) => p.replicaLag)} tone="text-success" start={start} />
      </div>
    </>
  );
}

/** Tiles built only from gauges the endpoint actually exposes. */
function gaugeTiles(snap: Snapshot) {
  const g = (n: string) => gauge(snap, n);
  const tiles: { label: string; value: string; unit?: string; hint?: string }[] = [];
  const size = g("beastdb_buffer_pool_size_pages");
  const cached = g("beastdb_buffer_pool_cached_pages");
  if (size !== undefined || cached !== undefined) {
    tiles.push({
      label: "Buffer pool",
      value: `${fmtInt(cached)}`,
      unit: size !== undefined ? `/ ${fmtInt(size)} pages` : "pages",
      hint: `pinned ${fmtInt(g("beastdb_buffer_pool_pinned_pages"))} · dirty ${fmtInt(g("beastdb_buffer_pool_dirty_pages"))}`,
    });
  }
  const wal = g("beastdb_wal_size_bytes");
  if (wal !== undefined) {
    const age = g("beastdb_checkpoint_age_seconds");
    tiles.push({
      label: "WAL size",
      value: formatBytes(wal),
      hint: [g("beastdb_wal_current_lsn") !== undefined ? `LSN ${fmtInt(g("beastdb_wal_current_lsn"))}` : "", age !== undefined ? `checkpoint ${Math.round(age)}s ago` : ""].filter(Boolean).join(" · "),
    });
  }
  const free = g("beastdb_disk_free_bytes");
  if (free !== undefined) tiles.push({ label: "Disk free", value: formatBytes(free), hint: g("beastdb_data_file_size_bytes") !== undefined ? `data file ${formatBytes(g("beastdb_data_file_size_bytes")!)}` : undefined });
  const up = g("beastdb_uptime_seconds");
  if (up !== undefined) {
    const h = Math.floor(up / 3600);
    tiles.push({ label: "Uptime", value: h >= 24 ? `${Math.floor(h / 24)}d ${h % 24}h` : h >= 1 ? `${h}h ${Math.floor((up % 3600) / 60)}m` : `${Math.floor(up / 60)}m ${Math.floor(up % 60)}s` });
  }
  // Fsync / group-commit stats: shown only if the server ever exports them.
  for (const fam of snap.values()) {
    if (!/fsync|group_commit/.test(fam.name) || fam.type === "histogram") continue;
    const v = gauge(snap, fam.name);
    if (v !== undefined) tiles.push({ label: fam.name.replace(/^beastdb_/, "").replace(/_/g, " "), value: fmtInt(v) });
  }
  return tiles;
}

export function MetricsView({ user }: ViewProps) {
  const admin = user?.role === "admin";
  const { status, snapshot, history, lastUpdated, error } = useMetricsHistory(admin);
  const [range, setRange] = useState<RangeId>("live");
  const rangeSeconds = RANGES.find((r) => r.id === range)?.seconds ?? 0;

  const header = (
    <PageHeader
      id="metrics-title"
      title="Metrics"
      subtitle="Live engine telemetry from /metrics, polled every 5 seconds."
      actions={
        <>
          {status === "ok" && lastUpdated && (
            <span className="inline-flex items-center gap-2 font-mono text-xs text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-primary" aria-hidden />
              live · {new Date(lastUpdated).toLocaleTimeString()}
            </span>
          )}
          {admin && status !== "forbidden" && (
            <select
              value={range}
              onChange={(e) => setRange(e.target.value as RangeId)}
              aria-label="Chart time range"
              className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {RANGES.map((r) => <option key={r.id} value={r.id}>{r.label}</option>)}
            </select>
          )}
        </>
      }
    />
  );

  let body: React.ReactNode;
  if (!admin || status === "forbidden") {
    body = <Notice icon={<Lock size={22} />} title="Admin only" body="Engine metrics are restricted to administrator sessions. Sign in as an admin to view them." />;
  } else if (status === "unavailable") {
    body = <Notice icon={<WifiOff size={22} />} title="Metrics unavailable" body={`Could not read /metrics${error ? `: ${error}` : ""}. Retrying every 5 seconds.`} />;
  } else if (status === "loading" || !snapshot) {
    body = <Notice icon={<Activity size={22} />} title="Loading metrics" body="Waiting for the first sample." />;
  } else {
    const reqRate = latest(history, "reqPerSec");
    const tiles = gaugeTiles(snapshot);
    body = (
      <>
        {error && <p role="alert" className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">Last poll failed ({error}); showing the previous sample.</p>}
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {has(history, "reqPerSec") ? (
            <MetricTile label="Requests / s" value={fmtRate(reqRate)} live={(reqRate ?? 0) > 0} />
          ) : (
            <MetricTile label="Requests / s" value={NA} hint="needs two samples" />
          )}
          {has(history, "errorRatio") && <MetricTile label="Error rate" value={fmtPct(latest(history, "errorRatio"))} hint={`${fmtRate(latest(history, "errPerSec"))} errors/s (5xx, server faults)`} />}
          {has(history, "p50") && <MetricTile label="Latency p50" value={fmtSeconds(latest(history, "p50"))} hint="last 5s window" />}
          {has(history, "p99") && <MetricTile label="Latency p99" value={fmtSeconds(latest(history, "p99"))} hint="bucket-interpolated" />}
          {has(history, "hitRatio") && <MetricTile label="Buffer hit ratio" value={fmtPct(latest(history, "hitRatio"))} hint="lifetime" />}
          {has(history, "inFlight") && <MetricTile label="gRPC in flight" value={fmtInt(latest(history, "inFlight"))} />}
          {tiles.map((t) => <MetricTile key={t.label} {...t} />)}
        </div>
        {rangeSeconds > 0 ? <HistoryCharts seconds={rangeSeconds} /> : (
        <div className="grid gap-4 lg:grid-cols-2">
          {has(history, "reqPerSec") && <ChartCard title="Requests / s" value={fmtRate(reqRate)} history={history} k="reqPerSec" />}
          {has(history, "errPerSec") && <ChartCard title="Errors / s" value={fmtRate(latest(history, "errPerSec"))} history={history} k="errPerSec" tone="text-destructive" />}
          {has(history, "p99") && <ChartCard title="Latency p99" value={fmtSeconds(latest(history, "p99"))} history={history} k="p99" tone="text-info" />}
          {has(history, "walBytes") && <ChartCard title="WAL size" value={formatBytes(latest(history, "walBytes") ?? 0)} history={history} k="walBytes" tone="text-warning" />}
          {has(history, "dirtyPages") && <ChartCard title="Dirty pages" value={fmtInt(latest(history, "dirtyPages"))} history={history} k="dirtyPages" tone="text-info" />}
        </div>
        )}
        <RawMetrics snapshot={snapshot} />
      </>
    );
  }

  return (
    <section aria-labelledby="metrics-title" className="space-y-6">
      {header}
      {body}
    </section>
  );
}
