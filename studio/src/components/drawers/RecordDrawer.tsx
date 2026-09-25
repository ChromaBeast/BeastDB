"use client";

import React, { useState } from "react";
import { X, Sparkles, Code2, Binary, Trash2, Database } from "lucide-react";
import { UniversalRecord } from "../../types";
import { FieldInspector } from "./FieldInspector";
import { RawPayloadViewer } from "./RawPayloadViewer";
import { KeyDetailsViewer } from "./KeyDetailsViewer";

interface RecordDrawerProps {
  record: UniversalRecord | null;
  onClose: () => void;
  onDelete: (key: number) => void;
}

export const RecordDrawer: React.FC<RecordDrawerProps> = ({ record, onClose, onDelete }) => {
  const [tab, setTab] = useState<"fields" | "raw" | "key">("fields");

  if (!record) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-sm transition-opacity">
      <div className="flex h-full w-full max-w-xl flex-col border-l border-white/10 bg-[#0c101b] p-6 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-purple-500/10 text-purple-400 ring-1 ring-purple-500/20">
              <Database className="h-4 w-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-semibold text-white truncate max-w-xs">{record.primaryLabel}</h2>
                <span className="rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-mono text-purple-300">
                  0x{record.prefix.toString(16).padStart(2, "0").toUpperCase()}
                </span>
              </div>
              <p className="font-mono text-[11px] text-slate-400">{record.prefixLabel}</p>
            </div>
          </div>
          <button onClick={onClose} className="rounded-lg p-1.5 text-slate-400 hover:bg-white/10 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* View Mode Navigation Tabs */}
        <div className="my-4 flex rounded-xl border border-white/[0.08] bg-black/30 p-1">
          <button
            onClick={() => setTab("fields")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium transition ${
              tab === "fields" ? "bg-purple-600/30 text-purple-300 shadow-sm" : "text-slate-400 hover:text-white"
            }`}
          >
            <Sparkles className="h-3.5 w-3.5" />
            <span>Field Inspector</span>
          </button>
          <button
            onClick={() => setTab("raw")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium transition ${
              tab === "raw" ? "bg-purple-600/30 text-purple-300 shadow-sm" : "text-slate-400 hover:text-white"
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            <span>Raw Payload</span>
          </button>
          <button
            onClick={() => setTab("key")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium transition ${
              tab === "key" ? "bg-purple-600/30 text-purple-300 shadow-sm" : "text-slate-400 hover:text-white"
            }`}
          >
            <Binary className="h-3.5 w-3.5" />
            <span>Key Bit-Slice</span>
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto pr-1">
          {tab === "fields" && <FieldInspector record={record} />}
          {tab === "raw" && <RawPayloadViewer raw={record.raw} byteSize={record.byteSize} />}
          {tab === "key" && <KeyDetailsViewer record={record} />}
        </div>

        {/* Footer Actions */}
        <div className="mt-4 flex items-center justify-between border-t border-white/[0.08] pt-4">
          <button
            onClick={() => {
              if (confirm(`Delete record key ${record.keyStr}?`)) {
                onDelete(record.key);
                onClose();
              }
            }}
            className="flex items-center gap-1.5 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs font-semibold text-red-400 transition hover:bg-red-500/20"
          >
            <Trash2 className="h-4 w-4" />
            <span>Delete Record</span>
          </button>
          <button
            onClick={onClose}
            className="rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-xs font-medium text-slate-300 transition hover:bg-white/[0.08] hover:text-white"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
