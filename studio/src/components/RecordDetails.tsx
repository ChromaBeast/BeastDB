"use client";
import { Copy, Edit2, Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { RecordValue } from "./RecordValue";
import { UserProfilePanel } from "./UserProfilePanel";
import { KeyBitSlicer } from "./KeyBitSlicer";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: () => void;
  onNotice: (message: string, error?: boolean) => void;
}

export function RecordDetails({ record, canWrite, onDelete, onDeleteUser, onEdit, onNotice }: Props) {
  if (record.prefix === 0x01) {
    return (
      <div className="flex-1 overflow-y-auto">
        <UserProfilePanel record={record} canWrite={canWrite} onDelete={onDelete} onDeleteUser={onDeleteUser ?? (() => {})} />
        <div className="px-5 pb-5">
          <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />
        </div>
      </div>
    );
  }

  const copy = async (text: string, label: string) => {
    try { await navigator.clipboard.writeText(text); onNotice(`${label} copied.`); }
    catch { onNotice("Copy failed.", true); }
  };

  const formatted = (() => {
    try { return JSON.stringify(JSON.parse(record.raw), null, 2); }
    catch { return record.raw; }
  })();

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-5 py-5">
      {/* Header with Title & Badges */}
      <div className="border-b border-border/70 pb-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="lime">{record.prefixLabel}</Badge>
            <Badge variant="mono">{record.format.replace("_", " ")}</Badge>
            <span className="text-xs font-mono text-zinc-500">{formatBytes(record.byteSize)}</span>
          </div>
          <div className="flex items-center gap-1.5">
            {canWrite && onEdit && (
              <Button variant="outline" size="sm" onClick={onEdit} className="h-7 text-xs">
                <Edit2 size={13} /> Edit
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={() => void copy(record.keyStr, "Decimal Key")} className="h-7 text-xs" title="Copy decimal key">
              <Copy size={13} /> Key
            </Button>
            {canWrite && (
              <Button variant="ghost" size="sm" onClick={() => onDelete(record.keyStr)} className="h-7 text-xs text-destructive hover:bg-destructive/10" title="Delete record">
                <Trash2 size={13} />
              </Button>
            )}
          </div>
        </div>
        <h2 className="mt-2 text-lg font-bold tracking-tight text-white">{record.primaryLabel}</h2>
        {record.secondaryLabel && <p className="text-xs text-zinc-400">{record.secondaryLabel}</p>}
        <p className="mt-1 font-mono text-xs text-zinc-500 break-all">Key: {record.keyStr} · Hex: {record.keyHex}</p>
      </div>

      {/* Cover Image Preview */}
      {record.coverUrl && (
        <img src={record.coverUrl} alt="" className="w-full max-h-48 rounded-lg object-cover border border-zinc-800" />
      )}

      {/* Visual Bit Slicer */}
      <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />

      {/* Structured Fields */}
      {record.fields && (
        <section className="space-y-2">
          <span className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">Record Fields</span>
          <div className="space-y-1.5 rounded-lg border border-border bg-card p-3">
            {Object.entries(record.fields).map(([k, v]) => (
              <div key={k} className="flex items-start justify-between gap-3 border-b border-zinc-800/60 pb-2 pt-1 last:border-b-0 last:pb-0">
                <span className="font-mono text-xs text-zinc-400 select-all">{k}</span>
                <div className="text-right text-xs"><RecordValue value={v} /></div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Array Items */}
      {record.arrayItems && (
        <section className="space-y-2">
          <span className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">Array Elements ({record.arrayItems.length})</span>
          <div className="space-y-1.5 rounded-lg border border-border bg-card p-3">
            {record.arrayItems.map((v, i) => (
              <div key={i} className="flex items-start justify-between gap-3 border-b border-zinc-800/60 pb-2 pt-1 last:border-b-0 last:pb-0">
                <span className="font-mono text-xs text-zinc-500">[{i}]</span>
                <div className="text-right text-xs"><RecordValue value={v} /></div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Raw Fallback */}
      {!record.fields && !record.arrayItems && (
        <div className="rounded-lg border border-border bg-card p-3 text-sm"><RecordValue value={record.raw || "Empty value"} /></div>
      )}

      {/* Raw JSON Code Viewer */}
      <details className="group rounded-lg border border-border bg-zinc-950/80">
        <summary className="flex cursor-pointer select-none items-center justify-between px-4 py-2.5 text-xs font-mono font-medium text-zinc-400 hover:text-white">
          <span>RAW JSON PAYLOAD</span>
          <span className="text-[10px] text-zinc-500">Click to expand</span>
        </summary>
        <div className="relative border-t border-zinc-800 p-3">
          <Button variant="ghost" size="icon" className="absolute right-3 top-3 h-6 w-6 text-zinc-400 hover:text-white" onClick={() => void copy(formatted, "Raw JSON")} title="Copy JSON">
            <Copy size={12} />
          </Button>
          <pre className="max-h-64 overflow-auto font-mono text-xs leading-5 text-zinc-300 whitespace-pre-wrap break-all">{formatted}</pre>
        </div>
      </details>
    </div>
  );
}
