"use client";

import React, { useState } from "react";
import { X, Binary, Calculator } from "lucide-react";
import { decodeKey } from "../../utils/key-decoder";
import { fnv1a64 } from "../../utils/format";

interface KeyCalculatorModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const KeyCalculatorModal: React.FC<KeyCalculatorModalProps> = ({
  isOpen,
  onClose,
}) => {
  const [inputVal, setInputVal] = useState("224236644308722058");
  const [stringHashInput, setStringHashInput] = useState("");

  if (!isOpen) return null;

  const decoded = decodeKey(inputVal.trim() || 0);

  const handleHashString = () => {
    if (stringHashInput.trim()) {
      const hashed = fnv1a64(stringHashInput.trim());
      setInputVal(String(hashed));
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-[#0d121f] p-6 shadow-2xl space-y-4">
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-cyan-600/20 text-cyan-400">
              <Binary className="h-4 w-4" />
            </div>
            <h2 className="text-sm font-semibold text-white">64-Bit Key Bit-Slicer & Analyzer</h2>
          </div>
          <button onClick={onClose} className="rounded-lg p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Key Input */}
        <div>
          <label className="text-xs font-semibold text-slate-300">Enter Numeric 64-bit Key</label>
          <input
            type="text"
            value={inputVal}
            onChange={(e) => setInputVal(e.target.value)}
            placeholder="e.g. 224236644308722058"
            className="mt-1 w-full rounded-xl border border-white/10 bg-black/50 p-2.5 font-mono text-xs text-purple-300 outline-none focus:border-purple-500"
          />
        </div>

        {/* String to Key Converter */}
        <div className="rounded-xl border border-white/[0.06] bg-black/30 p-3">
          <label className="text-[11px] font-medium text-slate-400">Or Hash String Identifier (FNV-1a 64)</label>
          <div className="mt-1 flex gap-2">
            <input
              type="text"
              placeholder="e.g. steam:1086940 or user@example.com"
              value={stringHashInput}
              onChange={(e) => setStringHashInput(e.target.value)}
              className="flex-1 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1 text-xs text-white outline-none focus:border-purple-500"
            />
            <button
              type="button"
              onClick={handleHashString}
              className="flex items-center gap-1 rounded-lg bg-purple-600 px-3 py-1 text-xs font-semibold text-white hover:bg-purple-500"
            >
              <Calculator className="h-3 w-3" /> Hash
            </button>
          </div>
        </div>

        {/* Bit Decomposition Results */}
        <div className="rounded-xl border border-white/[0.08] bg-black/40 p-4 space-y-3">
          <div className="flex items-center justify-between text-xs">
            <span className="text-slate-400">Hexadecimal:</span>
            <span className="font-mono text-purple-300 font-bold">{decoded.hex}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-slate-400">Partition Prefix (High 8 bits):</span>
            <span className="rounded bg-purple-500/20 px-2 py-0.5 font-mono text-purple-300 font-semibold">
              {decoded.prefixHex} ({decoded.prefixLabel})
            </span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-slate-400">User Scope Bucket (28 bits):</span>
            <span className="font-mono text-cyan-300">{decoded.userHash}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-slate-400">Item Index Hash (28 bits):</span>
            <span className="font-mono text-emerald-300">{decoded.itemHash}</span>
          </div>
        </div>

        <div className="flex justify-end pt-2 border-t border-white/[0.08]">
          <button
            onClick={onClose}
            className="rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-xs font-medium text-slate-300 hover:text-white"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
