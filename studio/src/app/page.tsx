"use client";

import React, { useState, useMemo } from "react";
import { Header } from "../components/Header";
import { StatsGrid } from "../components/StatsGrid";
import { TypeDistributionBar } from "../components/TypeDistributionBar";
import { RecordsToolbar } from "../components/RecordsToolbar";
import { RecordsTable } from "../components/RecordsTable";
import { RecordsGrid } from "../components/RecordsGrid";
import { RecordDrawer } from "../components/drawers/RecordDrawer";
import { NewRecordModal } from "../components/modals/NewRecordModal";
import { KeyCalculatorModal } from "../components/modals/KeyCalculatorModal";
import { UniversalRecord, ViewMode, PayloadFormat } from "../types";
import { useStudioData } from "../hooks/useStudioData";

export default function StudioDashboard() {
  const {
    stats,
    records,
    isLoading,
    isLoadingMore,
    hasMore,
    handleRefresh,
    handleLoadMore,
    handleSaveRecord,
    handleDeleteRecord,
  } = useStudioData();

  const [viewMode, setViewMode] = useState<ViewMode>("table");
  const [activePrefixFilter, setActivePrefixFilter] = useState<number | null>(null);
  const [formatFilter, setFormatFilter] = useState<PayloadFormat | "all">("all");
  const [search, setSearch] = useState("");
  const [activeRecord, setActiveRecord] = useState<UniversalRecord | null>(null);
  const [isNewModalOpen, setIsNewModalOpen] = useState(false);
  const [isKeyCalcOpen, setIsKeyCalcOpen] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const handleCopyKey = (keyStr: string) => {
    navigator.clipboard.writeText(keyStr);
    setCopiedKey(keyStr);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const onDeleteRecord = async (key: number) => {
    const success = await handleDeleteRecord(key);
    if (success && activeRecord?.key === key) {
      setActiveRecord(null);
    }
  };

  const filteredRecords = useMemo(() => {
    return records.filter((rec) => {
      if (activePrefixFilter !== null && rec.prefix !== activePrefixFilter) return false;
      if (formatFilter !== "all" && rec.format !== formatFilter) return false;
      if (!search.trim()) return true;

      const q = search.toLowerCase();
      return (
        rec.keyStr.includes(q) ||
        rec.primaryLabel.toLowerCase().includes(q) ||
        (rec.secondaryLabel && rec.secondaryLabel.toLowerCase().includes(q)) ||
        rec.raw.toLowerCase().includes(q)
      );
    });
  }, [records, activePrefixFilter, formatFilter, search]);

  return (
    <div className="min-h-screen bg-[#080B11] text-slate-100 selection:bg-purple-500/30 selection:text-purple-200">
      <Header
        stats={stats}
        onRefresh={handleRefresh}
        onOpenKeyCalculator={() => setIsKeyCalcOpen(true)}
        isLoading={isLoading}
      />

      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 space-y-6">
        <StatsGrid stats={stats} recordCount={records.length} />

        <TypeDistributionBar
          records={records}
          activePrefixFilter={activePrefixFilter}
          onSelectPrefix={setActivePrefixFilter}
        />

        <RecordsToolbar
          search={search}
          setSearch={setSearch}
          formatFilter={formatFilter}
          setFormatFilter={setFormatFilter}
          viewMode={viewMode}
          setViewMode={setViewMode}
          onOpenNew={() => setIsNewModalOpen(true)}
          hasMore={hasMore}
          onLoadMore={handleLoadMore}
          isLoadingMore={isLoadingMore}
        />

        {viewMode === "table" ? (
          <RecordsTable
            records={filteredRecords}
            onSelect={setActiveRecord}
            onDelete={onDeleteRecord}
            onCopyKey={handleCopyKey}
            copiedKey={copiedKey}
          />
        ) : (
          <RecordsGrid
            records={filteredRecords}
            onSelect={setActiveRecord}
            onCopyKey={handleCopyKey}
            copiedKey={copiedKey}
          />
        )}
      </main>

      <RecordDrawer
        record={activeRecord}
        onClose={() => setActiveRecord(null)}
        onDelete={onDeleteRecord}
      />

      <NewRecordModal
        isOpen={isNewModalOpen}
        onClose={() => setIsNewModalOpen(false)}
        onSave={handleSaveRecord}
      />

      <KeyCalculatorModal
        isOpen={isKeyCalcOpen}
        onClose={() => setIsKeyCalcOpen(false)}
      />
    </div>
  );
}
