"use client";
import { useEffect, useState } from "react";

/** One server-side sample from GET /api/metrics/history (see internal/metrics/history.go). */
export interface HistoryPoint {
  t: number; // unix seconds
  lsn: number;
  walBytes: number;
  dirtyPages: number;
  cachedPages: number;
  reqPerSec: number;
  replicaLag: number;
}

export interface ServerHistory {
  points: HistoryPoint[];
  intervalSeconds: number;
  windowSeconds: number;
}

const REFRESH_MS = 30_000;

/**
 * Loads the server's in-memory metric history for the last `windowSeconds`.
 * The server keeps it only in memory, so it is short right after a restart.
 * Pass enabled=false (non-admin) to skip fetching.
 */
export function useServerHistory(windowSeconds: number, enabled: boolean) {
  const [data, setData] = useState<ServerHistory | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const load = async () => {
      try {
        const res = await fetch(`/api/metrics/history?window=${windowSeconds}`, { cache: "no-store" });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = (await res.json()) as ServerHistory;
        if (cancelled) return;
        setData(body);
        setError(null);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "request failed");
      }
    };
    void load();
    const id = setInterval(() => void load(), REFRESH_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [windowSeconds, enabled]);

  return { data, error };
}
