"use client";

import React, { useState } from "react";
import { X, Sparkles, Code2, Plus, Wand2 } from "lucide-react";
import { fnv1a64 } from "../../utils/format";
import { DynamicFormBuilder, CustomFieldRow } from "./DynamicFormBuilder";
import { RawPayloadForm } from "./RawPayloadForm";

interface NewRecordModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (key: number, value: string) => Promise<boolean>;
}

export const NewRecordModal: React.FC<NewRecordModalProps> = ({
  isOpen,
  onClose,
  onSave,
}) => {
  const [tab, setTab] = useState<"builder" | "raw">("builder");
  const [keyInput, setKeyInput] = useState("");
  const [keySeed, setKeySeed] = useState("");
  const [fields, setFields] = useState<CustomFieldRow[]>([
    { id: "1", key: "title", type: "string", value: "New Item" },
    { id: "2", key: "status", type: "string", value: "active" },
    { id: "3", key: "score", type: "number", value: "9.5" },
  ]);
  const [rawValue, setRawValue] = useState('{\n  "title": "New Record",\n  "status": "active"\n}');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleGenerateKey = () => {
    if (keySeed.trim()) {
      setKeyInput(String(fnv1a64(keySeed.trim())));
    } else {
      const rand = Math.floor(Math.random() * 900000000000000) + 100000000000000;
      setKeyInput(String(rand));
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);

    let finalKey = Number(keyInput.trim());
    if (!finalKey || isNaN(finalKey)) {
      finalKey = fnv1a64(keySeed.trim() || `record-${Date.now()}`);
    }

    let payload = "";
    if (tab === "builder") {
      const obj: Record<string, any> = {};
      for (const f of fields) {
        if (!f.key.trim()) continue;
        if (f.type === "number") obj[f.key.trim()] = Number(f.value) || 0;
        else if (f.type === "boolean") obj[f.key.trim()] = f.value === "true";
        else if (f.type === "json") {
          try { obj[f.key.trim()] = JSON.parse(f.value); } catch { obj[f.key.trim()] = f.value; }
        } else {
          obj[f.key.trim()] = f.value;
        }
      }
      payload = JSON.stringify(obj, null, 2);
    } else {
      payload = rawValue;
    }

    const ok = await onSave(finalKey, payload);
    setIsSubmitting(false);
    if (ok) {
      setKeyInput("");
      setKeySeed("");
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-[#0d121f] p-6 shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-600/20 text-purple-400">
              <Plus className="h-4 w-4" />
            </div>
            <h2 className="text-sm font-semibold text-white">Insert BeastDB Record</h2>
          </div>
          <button onClick={onClose} className="rounded-lg p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Tab Switcher */}
        <div className="my-4 flex rounded-xl border border-white/[0.08] bg-black/30 p-1">
          <button
            type="button"
            onClick={() => setTab("builder")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium ${
              tab === "builder" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
          >
            <Sparkles className="h-3.5 w-3.5" /> Document Builder
          </button>
          <button
            type="button"
            onClick={() => setTab("raw")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium ${
              tab === "raw" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
          >
            <Code2 className="h-3.5 w-3.5" /> Raw Entry
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Key Generation Section */}
          <div className="rounded-xl border border-white/[0.06] bg-black/30 p-3 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-300">64-bit Numeric Key</span>
              <button
                type="button"
                onClick={handleGenerateKey}
                className="flex items-center gap-1 text-[11px] text-purple-400 hover:text-purple-300"
              >
                <Wand2 className="h-3 w-3" /> Auto Generate
              </button>
            </div>
            <div className="flex gap-2">
              <input
                type="number"
                placeholder="Enter 64-bit Key (or generate)"
                value={keyInput}
                onChange={(e) => setKeyInput(e.target.value)}
                className="flex-1 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1.5 font-mono text-xs text-white outline-none focus:border-purple-500"
              />
              <input
                type="text"
                placeholder="Key string seed (optional)"
                value={keySeed}
                onChange={(e) => setKeySeed(e.target.value)}
                className="w-40 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1.5 text-xs text-white outline-none focus:border-purple-500"
              />
            </div>
          </div>

          {tab === "builder" ? (
            <DynamicFormBuilder fields={fields} setFields={setFields} />
          ) : (
            <RawPayloadForm value={rawValue} setValue={setRawValue} />
          )}

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-white/[0.08]">
            <button
              type="button"
              onClick={onClose}
              className="rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-xs font-medium text-slate-300 hover:text-white"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="rounded-xl bg-purple-600 px-5 py-2 text-xs font-semibold text-white shadow-lg shadow-purple-600/30 hover:bg-purple-500 disabled:opacity-50"
            >
              {isSubmitting ? "Writing to B+ Tree..." : "Insert Record"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
