"use client";
import { useCallback, useEffect, useState } from "react";
import { PartitionConfig, SessionUser, TelemetryStats, UniversalRecord } from "../types";
import { parseUniversalRecord } from "../utils/data-parser";
import { setPartitionRegistry } from "../utils/key-decoder";

type RawRecord = { keyText: string; value: string };
type Page = { records: RawRecord[]; nextKeyText: string; hasMore: boolean };

async function api<T>(url: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(url, init);
  } catch {
    throw new Error("Cannot reach BeastDB. Check your connection and retry.");
  }
  if (response.status === 401)
    throw new Error("Your session expired. Sign in again.");
  if (response.status === 403)
    throw new Error("Your account cannot make this change.");
  if (response.status === 404)
    throw new Error(
      url.startsWith("/api/key?")
        ? "That record was not found."
        : "Data endpoint unavailable.",
    );
  if (!response.ok)
    throw new Error((await response.text()).trim() || "Request failed.");
  return response.status === 204 || response.status === 201
    ? (undefined as T)
    : response.json();
}

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
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  const fetchStats = useCallback(async () => {
    try {
      setStats(await api<TelemetryStats>("/api/stats?counts=true"));
      setStatsError(null);
    } catch (e) {
      setStats(null);
      setStatsError((e as Error).message);
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
    // Apply partition registry FIRST — decodeKey must see correct labels
    // before parseUniversalRecord is called below.
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
    await fetchStats();
    setLoading(false);
  }, [fetchStats]);

  useEffect(() => {
    void refresh();
  }, [refresh]);
  useEffect(() => {
    const timer = window.setInterval(() => {
      void fetchStats();
    }, 10000);
    return () => window.clearInterval(timer);
  }, [fetchStats]);

  const loadMore = async () => {
    if (!hasMore || loadingMore) return;
    setLoadingMore(true);
    try {
      const page = await api<Page>(`/api/records?start=${nextKey}&limit=100`);
      setRecords((current) => [
        ...current,
        ...page.records.map(parseUniversalRecord),
      ]);
      setHasMore(page.hasMore);
      setNextKey(page.nextKeyText);
      setError(null);
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
    await api<void>(`/api/key?k=${encodeURIComponent(key)}`, {
      method: "DELETE",
    });
    await refresh();
  };

  const removeUser = async (userHash: number): Promise<{ deleted: number }> => {
    const res = await api<{ deleted: number }>(`/api/user?userHash=${userHash}`, {
      method: "DELETE",
    });
    await refresh();
    return res;
  };

  const search = async (q: string, prefix?: number): Promise<UniversalRecord[]> => {
    const params = new URLSearchParams({ q, limit: "50" });
    if (prefix) params.set("prefix", String(prefix));
    const result = await api<{ records: RawRecord[]; count: number }>(`/api/search?${params}`);
    return result.records.map(parseUniversalRecord);
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
    updatedAt,
    refresh,
    loadMore,
    lookup,
    save,
    update,
    remove,
    removeUser,
    search,
  };
}
