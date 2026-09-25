"use client";

import React, { useState } from "react";
import { Copy, Check, ExternalLink, Image as ImageIcon, Calendar } from "lucide-react";
import { UniversalRecord } from "../../types";
import { inferFieldType } from "../../utils/data-parser";
import { formatDateRelative, formatDateFull } from "../../utils/format";

interface FieldInspectorProps {
  record: UniversalRecord;
}

export const FieldInspector: React.FC<FieldInspectorProps> = ({ record }) => {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const copyVal = (keyName: string, val: any) => {
    const text = typeof val === "object" ? JSON.stringify(val, null, 2) : String(val);
    navigator.clipboard.writeText(text);
    setCopiedKey(keyName);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const renderValue = (k: string, v: any) => {
    const type = inferFieldType(v);
    const isCopied = copiedKey === k;

    if (type === "image") {
      return (
        <div className="mt-1 space-y-2">
          <img
            src={String(v)}
            alt=""
            className="max-h-48 rounded-lg object-cover ring-1 ring-white/10"
            onError={(e) => { (e.target as HTMLElement).style.display = "none"; }}
          />
          <div className="flex items-center gap-1.5 font-mono text-[11px] text-slate-400">
            <span className="truncate max-w-xs">{String(v)}</span>
            <button onClick={() => copyVal(k, v)} className="hover:text-white">
              {isCopied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
            </button>
          </div>
        </div>
      );
    }

    if (type === "url") {
      return (
        <div className="flex items-center gap-1.5 text-xs text-purple-300">
          <a href={String(v)} target="_blank" rel="noreferrer" className="underline truncate hover:text-purple-200">
            {String(v)}
          </a>
          <ExternalLink className="h-3 w-3 shrink-0" />
        </div>
      );
    }

    if (type === "date") {
      return (
        <div className="flex items-center gap-1.5 text-xs text-slate-200">
          <Calendar className="h-3.5 w-3.5 text-slate-400" />
          <span>{formatDateFull(String(v))}</span>
          <span className="rounded bg-white/[0.04] px-1.5 py-0.5 text-[10px] text-slate-400">
            ({formatDateRelative(String(v))})
          </span>
        </div>
      );
    }

    if (type === "boolean") {
      return (
        <span className={`inline-block rounded px-2 py-0.5 text-xs font-semibold ${
          v ? "bg-emerald-500/20 text-emerald-400" : "bg-red-500/20 text-red-400"
        }`}>
          {String(v)}
        </span>
      );
    }

    if (type === "array") {
      return (
        <div className="flex flex-wrap gap-1.5 mt-1">
          {v.map((item: any, idx: number) => (
            <span key={idx} className="rounded-md bg-black/40 border border-white/10 px-2 py-0.5 text-xs text-slate-300">
              {typeof item === "object" ? JSON.stringify(item) : String(item)}
            </span>
          ))}
        </div>
      );
    }

    if (type === "object") {
      return (
        <div className="mt-1 rounded-xl border border-white/[0.06] bg-black/40 p-3 space-y-2">
          {Object.entries(v).map(([subK, subV]) => (
            <div key={subK} className="flex flex-col gap-0.5">
              <span className="text-[10px] font-mono uppercase text-slate-400">{subK}</span>
              <div className="text-xs text-slate-200 break-words">{renderValue(`${k}.${subK}`, subV)}</div>
            </div>
          ))}
        </div>
      );
    }

    return (
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-xs text-slate-200 break-words">{String(v)}</span>
        <button onClick={() => copyVal(k, v)} className="text-slate-400 hover:text-white shrink-0">
          {isCopied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
        </button>
      </div>
    );
  };

  if (!record.fields && !record.arrayItems) {
    return (
      <div className="rounded-xl border border-white/[0.08] bg-black/30 p-4">
        <span className="text-[10px] uppercase font-semibold text-slate-400">Raw Value</span>
        <div className="mt-2 font-mono text-xs text-purple-200 whitespace-pre-wrap break-all">
          {record.raw}
        </div>
      </div>
    );
  }

  const entries = record.fields
    ? Object.entries(record.fields)
    : (record.arrayItems || []).map((v, i) => [`[${i}]`, v]);

  return (
    <div className="space-y-3">
      {entries.map(([k, v]) => {
        const type = inferFieldType(v);
        return (
          <div key={k} className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-3.5">
            <div className="flex items-center justify-between mb-1">
              <span className="font-mono text-xs font-semibold text-purple-300">{k}</span>
              <span className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-[9px] uppercase tracking-wider text-slate-400">
                {type}
              </span>
            </div>
            <div className="mt-1">{renderValue(k, v)}</div>
          </div>
        );
      })}
    </div>
  );
};
