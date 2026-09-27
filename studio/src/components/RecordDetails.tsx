"use client";
import { Code2, Copy, Edit2, Link2, Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { RecordValue } from "./RecordValue";
import { UserProfilePanel } from "./UserProfilePanel";
import { KeyBitSlicer } from "./KeyBitSlicer";
import { fieldLabel, formatLabel } from "../utils/display";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: () => void;
  onNotice: (message: string, error?: boolean) => void;
}

export function RecordDetails({ record, canWrite, onDelete, onDeleteUser, onEdit, onNotice }: Props) {
  const copy = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      onNotice(`${label} copied.`);
    } catch {
      onNotice("Copy failed.", true);
    }
  };

  const copyShareLink = async () => {
    const url = new URL(window.location.href);
    url.searchParams.set("key", record.keyStr);
    url.searchParams.set("partition", String(record.prefix));
    await copy(url.toString(), "Shareable Link");
  };

  const formatted = (() => {
    try {
      return JSON.stringify(JSON.parse(record.raw), null, 2);
    } catch {
      return record.raw;
    }
  })();

  if (record.prefix === 0x01) {
    return (
      <div className="flex-1 overflow-y-auto">
        <UserProfilePanel record={record} canWrite={canWrite} onDelete={onDelete} onDeleteUser={onDeleteUser ?? (() => {})} />
        <div className="px-5 pb-5">
          <details className="group rounded-lg border border-border bg-card/50">
            <summary className="flex cursor-pointer select-none items-center justify-between px-4 py-2.5 text-xs font-mono font-medium text-zinc-400 hover:text-white">
              <span>KEY ANATOMY (64-BIT BIT SLICER)</span>
              <span className="text-[10px] text-zinc-500">Click to expand</span>
            </summary>
            <div className="p-3 border-t border-border">
              <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />
            </div>
          </details>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-5 py-5">
      {/* Header with Title, Badges & Actions */}
      <div className="border-b border-border/70 pb-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="lime">{record.prefixLabel}</Badge>
            <Badge variant="mono">{formatLabel(record.format)}</Badge>
            <span className="text-xs font-mono text-zinc-500">{formatBytes(record.byteSize)}</span>
          </div>
          <div className="flex items-center gap-1.5">
            {canWrite && onEdit && (
              <Button variant="outline" size="sm" onClick={onEdit} className="h-7 text-xs gap-1">
                <Edit2 size={13} /> Edit
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={() => void copy(record.keyStr, "Decimal Key")} className="h-7 text-xs gap-1" title="Copy decimal key">
              <Copy size={13} /> Key
            </Button>
            <Button variant="outline" size="sm" onClick={() => void copy(formatted, "JSON Payload")} className="h-7 text-xs gap-1" title="Copy JSON payload">
              <Code2 size={13} /> JSON
            </Button>
            <Button variant="outline" size="sm" onClick={() => void copyShareLink()} className="h-7 text-xs gap-1" title="Copy shareable URL link">
              <Link2 size={13} /> Link
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
      </div>

      {/* Cover Image Preview */}
      {record.coverUrl && (
        <img src={record.coverUrl} alt="" className="w-full max-h-48 rounded-lg object-cover border border-zinc-800" />
      )}

      {/* Structured Fields */}
      {record.fields && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-zinc-200">Details</h3>
          <div className="space-y-1.5 rounded-lg border border-border bg-card p-3">
            {Object.entries(record.fields).map(([k, v]) => (
              <div key={k} className="flex items-start justify-between gap-3 border-b border-zinc-800/60 pb-2 pt-1 last:border-b-0 last:pb-0">
                <span className="text-xs text-zinc-400 select-all">{fieldLabel(k)}</span>
                <div className="min-w-0 max-w-[65%] text-right text-sm"><RecordValue value={v} /></div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Array Items */}
      {record.arrayItems && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-zinc-200">Items ({record.arrayItems.length})</h3>
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
        <div className="rounded-lg border border-border bg-card p-3 text-sm">
          <RecordValue value={record.raw || "Empty value"} />
        </div>
      )}

      {/* Exact values remain available when needed. */}
      <details className="group rounded-lg border border-border bg-card/50">
        <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium text-zinc-300 hover:text-white">
          Technical details · key and raw value
        </summary>
        <div className="space-y-4 border-t border-border p-3">
          <div className="text-xs text-zinc-400">
            <p className="break-all">Key: <span className="font-mono text-zinc-200">{record.keyStr}</span></p>
            <p className="mt-1 break-all">Hex: <span className="font-mono text-zinc-200">{record.keyHex}</span></p>
          </div>
          <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />
          <div>
            <p className="mb-2 text-xs text-zinc-400">Raw value</p>
            <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-zinc-950 p-3 font-mono text-xs leading-5 text-zinc-300 whitespace-pre-wrap break-all">{formatted}</pre>
          </div>
        </div>
      </details>
    </div>
  );
}
