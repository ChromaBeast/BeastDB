"use client";

import React, { useState } from "react";
import { Check, AlertCircle } from "lucide-react";

interface RawPayloadFormProps {
  value: string;
  setValue: (v: string) => void;
}

export const RawPayloadForm: React.FC<RawPayloadFormProps> = ({ value, setValue }) => {
  const [isValidJson, setIsValidJson] = useState<boolean | null>(null);

  const checkJson = (text: string) => {
    setValue(text);
    if (!text.trim()) {
      setIsValidJson(null);
      return;
    }
    try {
      JSON.parse(text);
      setIsValidJson(true);
    } catch {
      setIsValidJson(false);
    }
  };

  const handlePrettify = () => {
    try {
      const parsed = JSON.parse(value);
      setValue(JSON.stringify(parsed, null, 2));
      setIsValidJson(true);
    } catch {
      // Ignore if not valid JSON
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-xs font-semibold text-slate-300">Raw Payload (JSON, Text, or Token) *</label>
        <div className="flex items-center gap-2">
          {isValidJson === true && (
            <span className="flex items-center gap-1 text-[10px] text-emerald-400">
              <Check className="h-3 w-3" /> Valid JSON
            </span>
          )}
          {isValidJson === false && (
            <span className="flex items-center gap-1 text-[10px] text-amber-400">
              <AlertCircle className="h-3 w-3" /> Plain Text / Non-JSON
            </span>
          )}
          <button
            type="button"
            onClick={handlePrettify}
            className="rounded bg-white/10 px-2 py-0.5 text-[10px] text-slate-300 hover:text-white"
          >
            Format JSON
          </button>
        </div>
      </div>
      <textarea
        rows={8}
        required
        value={value}
        onChange={(e) => checkJson(e.target.value)}
        placeholder='e.g. {"name": "Example", "count": 10} or custom token string'
        className="w-full rounded-xl border border-white/10 bg-black/50 p-3 font-mono text-xs text-purple-200 outline-none focus:border-purple-500 resize-none"
      />
    </div>
  );
};
