import { useState, useCallback, useEffect } from "react";
import { UniversalRecord, TelemetryStats } from "../types";
import { parseUniversalRecord } from "../utils/data-parser";

export function useStudioData() {
  const [stats, setStats] = useState<TelemetryStats | null>(null);
  const [records, setRecords] = useState<UniversalRecord[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextKey, setNextKey] = useState<number>(0);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch("/api/stats");
      if (res.ok) setStats(await res.json());
    } catch (e) {
      console.error("Telemetry error:", e);
    }
  }, []);

  const fetchRecords = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await fetch("/api/records?limit=150");
      if (res.ok) {
        const data = await res.json();
        const rawList: { key: number; value: string }[] = data.records || [];
        setRecords(rawList.map(parseUniversalRecord));
        setHasMore(Boolean(data.hasMore));
        setNextKey(Number(data.nextKey) || 0);
      }
    } catch (e) {
      console.error("Records error:", e);
    } finally {
      setIsLoading(false);
    }
  }, []);

  const handleLoadMore = async () => {
    if (!hasMore || isLoadingMore || !nextKey) return;
    setIsLoadingMore(true);
    try {
      const res = await fetch(`/api/records?start=${nextKey}&limit=100`);
      if (res.ok) {
        const data = await res.json();
        const rawList: { key: number; value: string }[] = data.records || [];
        setRecords((prev) => [...prev, ...rawList.map(parseUniversalRecord)]);
        setHasMore(Boolean(data.hasMore));
        setNextKey(Number(data.nextKey) || 0);
      }
    } finally {
      setIsLoadingMore(false);
    }
  };

  const handleRefresh = useCallback(() => {
    fetchStats();
    fetchRecords();
  }, [fetchStats, fetchRecords]);

  useEffect(() => {
    handleRefresh();
    const interval = setInterval(fetchStats, 10000);
    return () => clearInterval(interval);
  }, [handleRefresh, fetchStats]);

  const handleSaveRecord = async (key: number, value: string): Promise<boolean> => {
    try {
      const res = await fetch("/api/key", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ key, value }),
      });
      if (res.ok) {
        handleRefresh();
        return true;
      }
      alert("Failed to insert record.");
      return false;
    } catch {
      alert("Network error connecting to BeastDB engine.");
      return false;
    }
  };

  const handleDeleteRecord = async (key: number): Promise<boolean> => {
    try {
      const res = await fetch(`/api/key?k=${key}`, { method: "DELETE" });
      if (res.ok) {
        handleRefresh();
        return true;
      }
      alert("Failed to delete record.");
      return false;
    } catch {
      alert("Error deleting record.");
      return false;
    }
  };

  return {
    stats,
    records,
    isLoading,
    isLoadingMore,
    hasMore,
    handleRefresh,
    handleLoadMore,
    handleSaveRecord,
    handleDeleteRecord,
  };
}
