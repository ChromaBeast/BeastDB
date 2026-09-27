"use client";
import { useCallback, useEffect, useState } from "react";
import {
  PartitionConfig,
  SearchResponse,
  SessionUser,
  TelemetryStats,
  UniversalRecord,
} from "../types";
import { parseUniversalRecord } from "../utils/data-parser";
import { setPartitionRegistry } from "../utils/key-decoder";
import { api } from "../lib/api-client";

type RawRecord = { keyText: string; value: string };
type Page = { records: RawRecord[]; nextKeyText: string; hasMore: boolean };

export function useStudioData() {
  const [stats, setStats] = useState<TelemetryStats | null>(null);
  const [user, setUser] = useState<SessionUser | null>(null);
  const [records, setRecords] = useState<UniversalRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [statsError, setStatsError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [nextKey, setNextKey] = useState("0");
  const [activePartition, setActivePartition] = useState<number | null>(null);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  const fetchStats = useCallback(async (includeCounts = false) => {
    try {
      const url = includeCounts ? "/api/stats?counts=true" : "/api/stats";
      const s = await api<TelemetryStats>(url);
      setStats((prev) => (s.partitionCounts ? s : { ...s, partitionCounts: prev?.partitionCounts }));
      setStatsError(null);
    } catch (e) {
      setStats(null);
      setStatsError((e as Error).message);
    }
  }, []);

  const loadPartition = useCallback(async (prefix: number): Promise<void> => {
    setLoading(true);
    setError(null);
    setActivePartition(prefix);
    const startKey = (BigInt(prefix) << 56n).toString();
    const endKey = (((BigInt(prefix) + 1n) << 56n) - 1n).toString();
    try {
      const page = await api<Page>(`/api/records?start=${startKey}&end=${endKey}&limit=100`);
      setRecords(page.records.map(parseUniversalRecord));
      setHasMore(page.hasMore);
      setNextKey(page.nextKeyText);
      setUpdatedAt(new Date());
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    const [pageResult, userResult, partitionsResult] = await Promise.allSettled([
      api<Page>("/api/records?limit=100"),
      api<SessionUser>("/api/me"),
      api<PartitionConfig[]>("/api/partitions"),
    ]);
    if (partitionsResult.status === "fulfilled") {
      setPartitionRegistry(partitionsResult.value);
    }
    if (pageResult.status === "fulfilled") {
      setRecords(pageResult.value.records.map(parseUniversalRecord));
      setHasMore(pageResult.value.hasMore);
      setNextKey(pageResult.value.nextKeyText);
      setUpdatedAt(new Date());
    } else {
      setError(pageResult.reason.message);
    }
    if (userResult.status === "fulfilled") setUser(userResult.value);
    else {
      setUser(null);
      setError(userResult.reason.message);
    }
    await fetchStats(true);
    setLoading(false);
  }, [fetchStats]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchStats(false);
    }, 10000);
    return () => window.clearInterval(timer);
  }, [fetchStats]);

  const loadMore = async () => {
    if (!hasMore || loadingMore) return;
    setLoadingMore(true);
    try {
      let url = `/api/records?start=${nextKey}&limit=100`;
      if (activePartition !== null) {
        const endKey = (((BigInt(activePartition) + 1n) << 56n) - 1n).toString();
        url += `&end=${endKey}`;
      }
      const page = await api<Page>(url);
      setRecords((current) => [...current, ...page.records.map(parseUniversalRecord)]);
      setHasMore(page.hasMore);
      setNextKey(page.nextKeyText);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingMore(false);
    }
  };

  const lookup = async (key: string): Promise<UniversalRecord> => {
    const item = await api<RawRecord>(`/api/key?k=${encodeURIComponent(key)}`);
    return parseUniversalRecord(item);
  };

  const scanRange = async (startKey: string, endKey: string): Promise<UniversalRecord[]> => {
    const page = await api<Page>(`/api/records?start=${encodeURIComponent(startKey)}&end=${encodeURIComponent(endKey)}&limit=100`);
    return page.records.map(parseUniversalRecord);
  };

  const search = async (
    q: string,
    options?: { prefix?: number; start?: string; maxScan?: number },
  ): Promise<SearchResponse> => {
    const params = new URLSearchParams({ q, limit: "50" });
    if (options?.prefix) params.set("prefix", String(options.prefix));
    if (options?.start) params.set("start", options.start);
    if (options?.maxScan) params.set("maxScan", String(options.maxScan));
    const result = await api<{
      records: RawRecord[];
      count: number;
      scannedCount: number;
      nextKey?: string;
      hasMore: boolean;
      prefix?: number;
    }>(`/api/search?${params}`);
    return {
      records: result.records.map(parseUniversalRecord),
      count: result.count,
      scannedCount: result.scannedCount,
      nextKey: result.nextKey,
      hasMore: result.hasMore,
      prefix: result.prefix,
    };
  };

  const save = async (key: string, value: string): Promise<void> => {
    await api<void>("/api/key", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value, createOnly: true }),
    });
    await refresh();
  };

  const update = async (key: string, value: string): Promise<void> => {
    await api<void>("/api/key", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value, createOnly: false }),
    });
    await refresh();
  };

  const remove = async (key: string): Promise<void> => {
    await api<void>(`/api/key?k=${encodeURIComponent(key)}`, { method: "DELETE" });
    await refresh();
  };

  const removeUser = async (userHash: number): Promise<{ deleted: number }> => {
    const res = await api<{ deleted: number }>(`/api/user?userHash=${userHash}`, { method: "DELETE" });
    await refresh();
    return res;
  };

  return {
    stats,
    user,
    records,
    loading,
    loadingMore,
    error,
    statsError,
    hasMore,
    activePartition,
    updatedAt,
    refresh,
    loadMore,
    loadPartition,
    lookup,
    scanRange,
    save,
    update,
    remove,
    removeUser,
    search,
  };
}
