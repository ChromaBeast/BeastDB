"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import { Header } from "../components/Header";
import { StatsGrid } from "../components/StatsGrid";
import { RecordsTable } from "../components/RecordsTable";
import { RecordDrawer } from "../components/RecordDrawer";
import { NewRecordModal } from "../components/NewRecordModal";
import { DbRecord, TelemetryStats, CategoryFilter } from "../types";
import { enhanceRecord } from "../utils/format";

export default function StudioDashboard() {
  const [stats, setStats] = useState<TelemetryStats | null>(null);
  const [records, setRecords] = useState<DbRecord[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [filter, setFilter] = useState<CategoryFilter>("all");
  const [search, setSearch] = useState("");
  const [activeRecord, setActiveRecord] = useState<DbRecord | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch("/api/stats");
      if (res.ok) setStats(await res.json());
    } catch (e) {
      console.error("Telemetry fetch error:", e);
    }
  }, []);

  const fetchRecords = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await fetch("/api/records?limit=150");
      if (res.ok) {
        const data = await res.json();
        const rawList: { key: number; value: string }[] = data.records || [];
        setRecords(rawList.map(enhanceRecord));
      }
    } catch (e) {
      console.error("Records fetch error:", e);
    } finally {
      setIsLoading(false);
    }
  }, []);

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
      alert("Failed to insert record. Ensure key is valid.");
      return false;
    } catch {
      alert("Network error connecting to BeastDB engine.");
      return false;
    }
  };

  const handleDeleteRecord = async (key: number) => {
    try {
      const res = await fetch(`/api/key?k=${key}`, { method: "DELETE" });
      if (res.ok) {
        handleRefresh();
        if (activeRecord?.key === key) setActiveRecord(null);
      } else {
        alert("Failed to delete record.");
      }
    } catch {
      alert("Error deleting record.");
    }
  };

  const filteredRecords = useMemo(() => {
    return records.filter((rec) => {
      const recType = rec.parsed?.type?.toLowerCase() || (rec.value.startsWith("_sys") ? "system" : "custom");
      if (filter !== "all" && recType !== filter) return false;
      if (!search.trim()) return true;

      const q = search.toLowerCase();
      const keyStr = String(rec.key);
      const title = (rec.parsed?.title || rec.parsed?.name || "").toLowerCase();
      const valStr = rec.value.toLowerCase();
      return keyStr.includes(q) || title.includes(q) || valStr.includes(q);
    });
  }, [records, filter, search]);

  return (
    <div className="min-h-screen bg-[#080B11] text-slate-100 selection:bg-purple-500/30 selection:text-purple-200">
      <Header stats={stats} onRefresh={handleRefresh} isLoading={isLoading} />

      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 space-y-6">
        <StatsGrid stats={stats} recordCount={records.length} />

        <RecordsTable
          records={filteredRecords}
          filter={filter}
          setFilter={setFilter}
          search={search}
          setSearch={setSearch}
          onSelect={setActiveRecord}
          onDelete={handleDeleteRecord}
          onOpenNew={() => setIsModalOpen(true)}
        />
      </main>

      <RecordDrawer
        record={activeRecord}
        onClose={() => setActiveRecord(null)}
        onDelete={handleDeleteRecord}
      />

      <NewRecordModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSave={handleSaveRecord}
      />
    </div>
  );
}
