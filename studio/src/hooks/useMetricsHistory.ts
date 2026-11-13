"use client";
import { useEffect, useRef, useState } from "react";
import {
  gauge,
  histogram,
  histogramDelta,
  parsePrometheus,
  quantile,
  rate,
  sumSamples,
  type Histogram,
  type Snapshot,
} from "../utils/prometheus";

export const POLL_MS = 5000;
export const MAX_POINTS = 60;

export type MetricsStatus = "loading" | "ok" | "forbidden" | "unavailable";

/** One derived sample. A field is undefined when the endpoint does not expose its source series. */
export interface MetricsPoint {
  t: number;
  reqPerSec?: number;
  errPerSec?: number;
  errorRatio?: number;
  p50?: number;
  p99?: number;
  walBytes?: number;
  hitRatio?: number;
  dirtyPages?: number;
  inFlight?: number;
}

export type SeriesKey = Exclude<keyof MetricsPoint, "t">;

const REQ = "beastdb_requests_total";
const DUR = "beastdb_request_duration_seconds";
// Server-side failures only: HTTP 5xx and the gRPC codes that indicate a server fault.
const GRPC_SERVER_ERRORS = new Set(["Internal", "Unavailable", "Unknown", "DataLoss", "DeadlineExceeded"]);
const isErr = (code: string) => code.startsWith("5") || GRPC_SERVER_ERRORS.has(code);

interface Raw {
  t: number;
  req?: number;
  err?: number;
  hist?: Histogram;
  hits?: number;
  misses?: number;
}

function readRaw(snap: Snapshot, t: number): Raw {
  return {
    t,
    req: sumSamples(snap, REQ),
    err: sumSamples(snap, REQ, undefined, (s) => isErr(s.labels.code ?? "")),
    hist: histogram(snap, DUR),
    hits: sumSamples(snap, "beastdb_buffer_pool_hits_total"),
    misses: sumSamples(snap, "beastdb_buffer_pool_misses_total"),
  };
}

function derive(cur: Raw, prev: Raw | undefined, snap: Snapshot): MetricsPoint {
  const dt = prev ? (cur.t - prev.t) / 1000 : 0;
  const reqPerSec = prev ? rate(cur.req, prev.req, dt) : undefined;
  const errPerSec = prev ? rate(cur.err, prev.err, dt) : undefined;
  let errorRatio: number | undefined;
  if (reqPerSec !== undefined && errPerSec !== undefined && reqPerSec > 0) errorRatio = errPerSec / reqPerSec;
  else if (reqPerSec === 0) errorRatio = 0;
  // Latency over the last interval only (windowed), so it tracks current load rather than lifetime.
  const win = cur.hist && prev?.hist ? histogramDelta(cur.hist, prev.hist) : undefined;
  return {
    t: cur.t,
    reqPerSec,
    errPerSec,
    errorRatio,
    p50: win ? quantile(win, 0.5) : undefined,
    p99: win ? quantile(win, 0.99) : undefined,
    walBytes: gauge(snap, "beastdb_wal_size_bytes"),
    hitRatio: gauge(snap, "beastdb_buffer_pool_hit_ratio"),
    dirtyPages: gauge(snap, "beastdb_buffer_pool_dirty_pages"),
    inFlight: gauge(snap, "beastdb_grpc_requests_in_flight"),
  };
}

export interface MetricsState {
  status: MetricsStatus;
  snapshot: Snapshot | null;
  history: MetricsPoint[];
  lastUpdated: number | null;
  error: string | null;
}

/** Polls /metrics while mounted and keeps a rolling in-memory history with client-side rates. */
export function useMetricsHistory(enabled: boolean): MetricsState {
  const [state, setState] = useState<MetricsState>({
    status: "loading",
    snapshot: null,
    history: [],
    lastUpdated: null,
    error: null,
  });
  const prevRaw = useRef<Raw | undefined>(undefined);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const tick = async () => {
      try {
        const res = await fetch("/metrics", { cache: "no-store" });
        if (cancelled) return;
        if (res.status === 401 || res.status === 403) {
          setState((s) => ({ ...s, status: "forbidden", error: null }));
          return; // no point polling a forbidden endpoint
        }
        if (!res.ok) throw new Error(`/metrics returned HTTP ${res.status}`);
        const snap = parsePrometheus(await res.text());
        if (cancelled) return;
        if (snap.size === 0) throw new Error("/metrics returned no samples");
        const raw = readRaw(snap, Date.now());
        const point = derive(raw, prevRaw.current, snap);
        prevRaw.current = raw;
        setState((s) => ({
          status: "ok",
          snapshot: snap,
          history: [...s.history, point].slice(-MAX_POINTS),
          lastUpdated: raw.t,
          error: null,
        }));
      } catch (e) {
        if (cancelled) return;
        setState((s) => ({ ...s, status: s.snapshot ? s.status : "unavailable", error: (e as Error).message }));
      }
      if (!cancelled) timer = setTimeout(tick, POLL_MS);
    };
    void tick();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [enabled]);

  return state;
}
