"use client";
import { useState } from "react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";

interface Props {
  record: UniversalRecord;
  onSave: (key: string, value: string) => Promise<void>;
  onCancel: () => void;
  onNotice: (message: string, error?: boolean) => void;
}

export function EditRecordPanel({ record, onSave, onCancel, onNotice }: Props) {
  const formatted = (() => {
    try { return JSON.stringify(JSON.parse(record.raw), null, 2); } catch { return record.raw; }
  })();
  const [value, setValue] = useState(formatted);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    const trimmed = value.trim();
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      try { JSON.parse(trimmed); } catch {
        onNotice("Invalid JSON — fix syntax errors before saving.", true);
        return;
      }
    }
    setSaving(true);
    try {
      await onSave(record.keyStr, value);
      onNotice("Record saved.");
    } catch (e) {
      onNotice((e as Error).message, true);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-4 p-5">
      <div>
        <h2 className="text-base font-semibold">Edit Record</h2>
        <p className="mt-0.5 break-all font-mono text-xs text-muted-foreground">{record.keyStr}</p>
      </div>
      <textarea
        className="min-h-[300px] w-full rounded-md border bg-muted/30 p-3 font-mono text-xs leading-6 focus:outline-none focus:ring-2 focus:ring-ring"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        aria-label="Record value"
        spellCheck={false}
      />
      <p className="text-xs text-muted-foreground">{value.length} characters</p>
      <div className="flex gap-2">
        <Button variant="outline" onClick={onCancel} disabled={saving}>Cancel</Button>
        <Button onClick={() => void handleSave()} disabled={saving}>
          {saving ? "Saving…" : "Save"}
        </Button>
      </div>
    </div>
  );
}
