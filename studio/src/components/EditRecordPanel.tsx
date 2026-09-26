"use client";
import { useState } from "react";
import { Check, Code2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";

interface Props {
  record: UniversalRecord;
  onSave: (key: string, value: string) => Promise<void>;
  onCancel: () => void;
  onNotice: (message: string, error?: boolean) => void;
}

export function EditRecordPanel({ record, onSave, onCancel, onNotice }: Props) {
  const initialFormatted = (() => {
    try { return JSON.stringify(JSON.parse(record.raw), null, 2); } catch { return record.raw; }
  })();
  const [value, setValue] = useState(initialFormatted);
  const [saving, setSaving] = useState(false);

  const formatJson = () => {
    try {
      setValue(JSON.stringify(JSON.parse(value), null, 2));
      onNotice("JSON formatted.", false);
    } catch {
      onNotice("Cannot format: Value is not valid JSON.", true);
    }
  };

  const handleSave = async () => {
    const trimmed = value.trim();
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      try { JSON.parse(trimmed); } catch {
        onNotice("Invalid JSON payload — fix syntax errors before committing.", true);
        return;
      }
    }
    setSaving(true);
    try {
      await onSave(record.keyStr, value);
      onNotice("Record updated successfully.");
    } catch (e) {
      onNotice((e as Error).message, true);
    } finally {
      setSaving(false);
    }
  };

  const byteLen = new TextEncoder().encode(value).length;

  return (
    <div className="flex flex-col gap-4 p-5 bg-[#0c0c0e] rounded-xl">
      <div className="flex items-center justify-between border-b border-zinc-800 pb-3">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-base font-bold text-white">Edit Record</h2>
            <Badge variant="lime">{record.prefixLabel}</Badge>
          </div>
          <p className="mt-1 font-mono text-xs text-zinc-500 break-all">Key: {record.keyStr}</p>
        </div>
        <Button variant="outline" size="sm" onClick={formatJson} className="h-7 text-xs gap-1.5" title="Prettify JSON indentation">
          <Code2 size={13} /> Prettify
        </Button>
      </div>

      <textarea
        className="min-h-[320px] w-full rounded-lg border border-zinc-800 bg-zinc-950 p-3.5 font-mono text-xs leading-5 text-zinc-200 focus:outline-none focus:ring-2 focus:ring-beast-lime"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        aria-label="Record payload"
        spellCheck={false}
      />

      <div className="flex items-center justify-between text-xs text-zinc-500 font-mono">
        <span>{value.length} characters · {formatBytes(byteLen)}</span>
        <span>UTF-8 Slotted Page Payload</span>
      </div>

      <div className="flex justify-end gap-2 border-t border-zinc-800 pt-3">
        <Button variant="outline" onClick={onCancel} disabled={saving} className="text-xs">
          Cancel
        </Button>
        <Button onClick={() => void handleSave()} disabled={saving} className="text-xs gap-1.5">
          <Check size={14} /> {saving ? "Writing to WAL…" : "Commit Changes"}
        </Button>
      </div>
    </div>
  );
}
