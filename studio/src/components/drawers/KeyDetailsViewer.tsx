"use client";

import React, { useState } from "react";
import { Copy, Check, Binary, Key } from "lucide-react";
import { UniversalRecord } from "../../types";

interface KeyDetailsViewerProps {
  record: UniversalRecord;
}

export const KeyDetailsViewer: React.FC<KeyDetailsViewerProps> = ({ record }) => {
  const [copiedType, setCopiedType] = useState<string | null>(null);

  const copy = (val: string, type: string) => {
    navigator.clipboard.writeText(val);
    setCopiedType(type);
    setTimeout(() => setCopiedType(null), 2000);
  };

  const binaryStr = (() => {
    try {
      return BigInt(record.keyStr).toString(2).padStart(64, "0");
    } catch {
      return "0".repeat(64);
    }
  })();

  const prefixBits = binaryStr.slice(0, 8);
  const userBits = binaryStr.slice(8, 36);
  const itemBits = binaryStr.slice(36, 64);

  return (
    <div className="space-y-4">
      {/* Overview Card */}
      <div className="rounded-xl border border-purple-500/20 bg-purple-500/5 p-4">
        <div className="flex items-center gap-2 text-purple-300">
          <Key className="h-4 w-4" />
          <span className="text-xs font-semibold uppercase tracking-wider">64-bit B+ Tree Key</span>
        </div>
        <div className="mt-2 flex items-baseline justify-between gap-2">
          <span className="font-mono text-base font-bold text-white break-all">{record.keyStr}</span>
          <button
            onClick={() => copy(record.keyStr, "dec")}
            className="flex items-center gap-1 rounded bg-white/10 px-2 py-0.5 text-[10px] text-slate-300 hover:text-white"
          >
            {copiedType === "dec" ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
            <span>Dec</span>
          </button>
        </div>
        <div className="mt-1 flex items-center justify-between text-xs text-slate-400 font-mono">
          <span>{record.keyHex}</span>
          <button
            onClick={() => copy(record.keyHex, "hex")}
            className="hover:text-white text-[10px]"
          >
            {copiedType === "hex" ? "Copied" : "Copy Hex"}
          </button>
        </div>
      </div>

      {/* Bit Slicing Visualizer */}
      <div className="rounded-xl border border-white/[0.08] bg-black/40 p-4">
        <div className="flex items-center gap-2 mb-3">
          <Binary className="h-4 w-4 text-cyan-400" />
          <span className="text-xs font-semibold text-white">64-Bit Binary Decomposition</span>
        </div>

        {/* Binary Bar with Colors */}
        <div className="font-mono text-[11px] leading-relaxed break-all bg-black/60 p-3 rounded-lg border border-white/[0.06]">
          <span className="text-purple-400 bg-purple-500/20 px-1 py-0.5 rounded" title="Domain Prefix (8 bits)">
            {prefixBits}
          </span>
          <span className="text-slate-500 mx-1">•</span>
          <span className="text-cyan-400 bg-cyan-500/20 px-1 py-0.5 rounded" title="User Hash (28 bits)">
            {userBits}
          </span>
          <span className="text-slate-500 mx-1">•</span>
          <span className="text-emerald-400 bg-emerald-500/20 px-1 py-0.5 rounded" title="Item Hash (28 bits)">
            {itemBits}
          </span>
        </div>

        {/* Legend */}
        <div className="mt-4 grid grid-cols-3 gap-2 text-xs">
          <div className="rounded-lg border border-purple-500/20 bg-purple-500/5 p-2.5">
            <span className="text-[10px] uppercase font-semibold text-purple-400">Prefix (8 bits)</span>
            <div className="mt-1 font-mono text-xs font-bold text-white">
              0x{record.prefix.toString(16).padStart(2, "0").toUpperCase()}
            </div>
            <div className="mt-0.5 text-[10px] text-slate-400 truncate">{record.prefixLabel}</div>
          </div>

          <div className="rounded-lg border border-cyan-500/20 bg-cyan-500/5 p-2.5">
            <span className="text-[10px] uppercase font-semibold text-cyan-400">Bucket (28 bits)</span>
            <div className="mt-1 font-mono text-xs font-bold text-white truncate">
              {record.userHash !== undefined ? record.userHash : "N/A"}
            </div>
            <div className="mt-0.5 text-[10px] text-slate-400">User Scope Hash</div>
          </div>

          <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-2.5">
            <span className="text-[10px] uppercase font-semibold text-emerald-400">Item (28 bits)</span>
            <div className="mt-1 font-mono text-xs font-bold text-white truncate">
              {record.itemHash !== undefined ? record.itemHash : "N/A"}
            </div>
            <div className="mt-0.5 text-[10px] text-slate-400">Item ID Hash</div>
          </div>
        </div>
      </div>
    </div>
  );
};
