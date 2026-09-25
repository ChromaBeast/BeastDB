"use client";

import React from "react";
import { UniversalRecord } from "../types";
import { RecordCard } from "./RecordCard";
import { Database } from "lucide-react";

interface RecordsGridProps {
  records: UniversalRecord[];
  onSelect: (record: UniversalRecord) => void;
  onCopyKey: (keyStr: string) => void;
  copiedKey: string | null;
}

export const RecordsGrid: React.FC<RecordsGridProps> = ({
  records,
  onSelect,
  onCopyKey,
  copiedKey,
}) => {
  if (records.length === 0) {
    return (
      <div className="rounded-2xl border border-white/[0.08] bg-white/[0.02] p-12 text-center text-slate-400 backdrop-blur-xl">
        <Database className="mx-auto mb-3 h-10 w-10 text-slate-600 opacity-60" />
        <p className="text-sm">No records match your query or partition filters.</p>
        <p className="mt-1 text-xs text-slate-500">Try adjusting your search terms or selecting All Records.</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {records.map((rec) => (
        <RecordCard
          key={rec.keyStr}
          record={rec}
          onSelect={onSelect}
          onCopyKey={onCopyKey}
          isCopied={copiedKey === rec.keyStr}
        />
      ))}
    </div>
  );
};
