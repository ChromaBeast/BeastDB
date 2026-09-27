"use client";
import { Code2, Copy, Link2, Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { formatLabel } from "../utils/display";
import { Button } from "./ui/button";
import { KeyBitSlicer } from "./KeyBitSlicer";
import { RecordFieldList } from "./RecordFieldList";

interface Props {
  record: UniversalRecord;
  formattedRaw: string;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onNotice: (message: string, error?: boolean) => void;
}

export function RecordEngineDetails({ record, formattedRaw, canWrite, onDelete, onNotice }: Props) {
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

  return (
    <details className="group rounded-lg border border-border bg-card/40 text-xs">
      <summary className="flex cursor-pointer select-none items-center justify-between px-4 py-2.5 font-mono text-muted-foreground hover:text-foreground hover:bg-muted/40 rounded-lg">
        <span>ENGINE & STORAGE INTERNALS</span>
        <span className="text-[10px] opacity-70">Click to expand</span>
      </summary>
      <div className="space-y-3.5 border-t border-border p-3.5">
        {record.fields && <RecordFieldList fields={record.fields} identifiers />}
        <p className="text-muted-foreground">{formatLabel(record.format)} · {formatBytes(record.byteSize)}</p>
        <div className="font-mono text-muted-foreground space-y-1">
          <p className="break-all">Key: <span className="text-foreground font-semibold">{record.keyStr}</span></p>
          <p className="break-all">Hex: <span className="text-foreground font-semibold">{record.keyHex}</span></p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => void copy(record.keyStr, "Key")}><Copy size={12} /> Key</Button>
          <Button variant="outline" size="sm" onClick={() => void copy(formattedRaw, "Raw")}><Code2 size={12} /> JSON</Button>
          <Button variant="outline" size="sm" onClick={() => void copyShareLink()}><Link2 size={12} /> Link</Button>
          {canWrite && <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10" onClick={() => onDelete(record.keyStr)}><Trash2 size={12} /> Delete</Button>}
        </div>
        <KeyBitSlicer keyStr={record.keyStr} prefixLabel={record.prefixLabel} userHash={record.userHash} itemHash={record.itemHash} onNotice={onNotice} />
        <div>
          <p className="mb-1.5 text-muted-foreground font-semibold">Raw Storage Payload</p>
          <pre className="max-h-56 overflow-auto rounded-lg border border-border bg-muted/40 p-2.5 font-mono text-[11px] leading-relaxed text-foreground whitespace-pre-wrap break-all">{formattedRaw}</pre>
        </div>
      </div>
    </details>
  );
}
