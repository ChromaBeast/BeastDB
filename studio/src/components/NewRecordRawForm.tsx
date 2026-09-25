"use client";

import React from "react";

interface NewRecordRawFormProps {
  rawKey: string;
  setRawKey: (v: string) => void;
  rawValue: string;
  setRawValue: (v: string) => void;
}

export const NewRecordRawForm: React.FC<NewRecordRawFormProps> = ({
  rawKey,
  setRawKey,
  rawValue,
  setRawValue,
}) => {
  return (
    <>
      <div>
        <label className="text-[11px] font-medium text-slate-400">Key (Numeric uint64) *</label>
        <input
          type="number"
          required
          placeholder="e.g. 1001"
          value={rawKey}
          onChange={(e) => setRawKey(e.target.value)}
          className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500 font-mono"
        />
      </div>
      <div>
        <label className="text-[11px] font-medium text-slate-400">Value (Payload / JSON) *</label>
        <textarea
          rows={6}
          required
          value={rawValue}
          onChange={(e) => setRawValue(e.target.value)}
          className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 p-2.5 text-xs text-purple-200 outline-none focus:border-purple-500 font-mono resize-none"
        />
      </div>
    </>
  );
};
