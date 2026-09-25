"use client";

import React, { useState } from "react";
import { X, Copy, Check, Trash2, Code2, Sparkles, Database } from "lucide-react";
import { DbRecord } from "../types";

interface RecordDrawerProps {
  record: DbRecord | null;
  onClose: () => void;
  onDelete: (key: number) => void;
}

export const RecordDrawer: React.FC<RecordDrawerProps> = ({ record, onClose, onDelete }) => {
  const [copied, setCopied] = useState(false);
  const [viewMode, setViewMode] = useState<"visual" | "raw">("visual");

  if (!record) return null;

  const handleCopy = () => {
    navigator.clipboard.writeText(record.value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const formattedJson = (() => {
    try {
      return JSON.stringify(JSON.parse(record.value), null, 2);
    } catch {
      return record.value;
    }
  })();

  const parsed = record.parsed;

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-sm transition-opacity">
      <div className="flex h-full w-full max-w-xl flex-col border-l border-white/10 bg-[#0c101b] p-6 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-4">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-500/10 text-purple-400">
              <Database className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-white">Record Details</h2>
              <p className="font-mono text-xs text-slate-400">Key: {record.key}</p>
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            <button
              onClick={handleCopy}
              className="flex items-center gap-1 rounded-lg border border-white/10 bg-white/[0.04] px-2.5 py-1 text-xs text-slate-300 hover:text-white"
              title="Copy payload"
            >
              {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
              <span>{copied ? "Copied" : "Copy"}</span>
            </button>
            <button
              onClick={onClose}
              className="rounded-lg p-1 text-slate-400 hover:bg-white/10 hover:text-white"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* View Mode Toggle */}
        <div className="my-4 flex rounded-xl border border-white/[0.08] bg-black/30 p-1">
          <button
            onClick={() => setViewMode("visual")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium transition ${
              viewMode === "visual" ? "bg-purple-600/30 text-purple-300 shadow-sm" : "text-slate-400 hover:text-white"
            }`}
          >
            <Sparkles className="h-3.5 w-3.5" />
            <span>Human Inspector</span>
          </button>
          <button
            onClick={() => setViewMode("raw")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium transition ${
              viewMode === "raw" ? "bg-purple-600/30 text-purple-300 shadow-sm" : "text-slate-400 hover:text-white"
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            <span>Raw JSON Data</span>
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto pr-1">
          {viewMode === "visual" && parsed ? (
            <div className="space-y-4">
              {/* Primary Entity Banner */}
              <div className="rounded-xl border border-purple-500/20 bg-gradient-to-br from-purple-500/10 to-indigo-500/5 p-4">
                <div className="flex items-center justify-between">
                  <span className="rounded bg-purple-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-purple-300">
                    {parsed.type ?? "ENTITY"}
                  </span>
                  {parsed.rating && (
                    <span className="text-xs font-semibold text-amber-400">
                      ★ {parsed.rating} / 10
                    </span>
                  )}
                </div>
                <h3 className="mt-2 text-lg font-bold text-white">
                  {parsed.title || parsed.name || "Untitled Entity"}
                </h3>
                {parsed.status && (
                  <p className="mt-1 text-xs text-emerald-400 font-medium">Status: {parsed.status}</p>
                )}
                {parsed.description && (
                  <p className="mt-2 text-xs leading-relaxed text-slate-300">{parsed.description}</p>
                )}
              </div>

              {/* Parsed Fields Grid */}
              <div className="grid grid-cols-2 gap-3">
                {Object.entries(parsed)
                  .filter(([k]) => !["title", "name", "description", "type", "status", "rating"].includes(k))
                  .map(([k, v]) => (
                    <div key={k} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
                      <div className="text-[10px] uppercase font-semibold text-slate-400">{k}</div>
                      <div className="mt-1 font-mono text-xs text-white truncate">
                        {typeof v === "object" ? JSON.stringify(v) : String(v)}
                      </div>
                    </div>
                  ))}
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-white/[0.08] bg-black/50 p-4">
              <pre className="font-mono text-xs leading-relaxed text-purple-200/90 whitespace-pre-wrap break-all">
                {formattedJson}
              </pre>
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="mt-4 flex items-center justify-between border-t border-white/[0.08] pt-4">
          <button
            onClick={() => {
              if (confirm(`Delete record key ${record.key}?`)) {
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
