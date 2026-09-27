"use client";
import { Code2, Copy, Edit2, Link2, Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { RecordValue } from "./RecordValue";
import { RecordFieldList } from "./RecordFieldList";
import { UserProfilePanel } from "./UserProfilePanel";
import { KeyBitSlicer } from "./KeyBitSlicer";
import { formatLabel } from "../utils/display";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: () => void;
  onNotice: (message: string, error?: boolean) => void;
  onScanRange?: (start: string, end: string) => Promise<UniversalRecord[]>;
  onSelect?: (record: UniversalRecord) => void;
}

export function RecordDetails({ record, canWrite, onDelete, onDeleteUser, onEdit, onNotice, onScanRange, onSelect }: Props) {
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

  const noop = async () => [] as UniversalRecord[];
  if (record.prefix === 0x01) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <UserProfilePanel
          record={record} canWrite={canWrite}
          onDelete={onDelete} onDeleteUser={onDeleteUser ?? (() => {})}
          onScanRange={onScanRange ?? noop} onSelect={onSelect ?? (() => {})}
        />
        <div className="px-5 pb-5">
          <details className="group rounded-lg border border-border bg-card/50">
            <summary className="flex cursor-pointer select-none items-center justify-between px-4 py-2.5 text-xs font-mono font-medium text-muted-foreground hover:text-foreground">
              <span>KEY ANATOMY (64-BIT BIT SLICER)</span>
              <span className="text-[10px] text-muted-foreground">Click to expand</span>
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
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-5">
      {/* Content first; storage controls are below. */}
      <div className="border-b border-border/70 pb-4">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate text-xs text-muted-foreground">{record.prefixLabel}</span>
          {canWrite && onEdit && (
            <Button variant="outline" size="sm" onClick={onEdit} className="h-7 shrink-0 gap-1 text-xs">
              <Edit2 size={13} /> Edit
            </Button>
          )}
        </div>
        <h2 className="mt-2 text-lg font-bold tracking-tight text-foreground">{record.primaryLabel}</h2>
        {record.secondaryLabel && <p className="text-xs text-muted-foreground">{record.secondaryLabel}</p>}
      </div>

      {/* Cover Image Preview */}
      {record.coverUrl && (
        <img src={record.coverUrl} alt="" className="w-full max-h-48 rounded-lg object-cover border border-border" />
      )}

      {/* Structured Fields */}
      {record.fields && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-foreground">Details</h3>
          <RecordFieldList fields={record.fields} />
        </section>
      )}

      {/* Array Items */}
      {record.arrayItems && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold text-foreground">Items ({record.arrayItems.length})</h3>
          <div className="rounded-lg border border-border bg-card p-3 text-sm"><RecordValue value={record.arrayItems} /></div>
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
        <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium text-muted-foreground hover:text-foreground">
          Technical details
        </summary>
        <div className="space-y-4 border-t border-border p-3">
          {record.fields && <RecordFieldList fields={record.fields} identifiers />}
          <p className="text-xs text-muted-foreground">{formatLabel(record.format)} · {formatBytes(record.byteSize)}</p>
          <div className="text-xs text-muted-foreground">
            <p className="break-all">Key: <span className="font-mono text-foreground">{record.keyStr}</span></p>
            <p className="mt-1 break-all">Hex: <span className="font-mono text-foreground">{record.keyHex}</span></p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => void copy(record.keyStr, "Key")}><Copy size={13} /> Copy key</Button>
            <Button variant="outline" size="sm" onClick={() => void copy(formatted, "Raw value")}><Code2 size={13} /> Copy raw</Button>
            <Button variant="outline" size="sm" onClick={() => void copyShareLink()}><Link2 size={13} /> Copy link</Button>
            {canWrite && <Button variant="ghost" size="sm" className="text-destructive" onClick={() => onDelete(record.keyStr)}><Trash2 size={13} /> Delete</Button>}
          </div>
          <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />
          <div>
            <p className="mb-2 text-xs text-muted-foreground">Raw value</p>
            <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs leading-5 text-foreground whitespace-pre-wrap break-all">{formatted}</pre>
          </div>
        </div>
      </details>
    </div>
  );
}
