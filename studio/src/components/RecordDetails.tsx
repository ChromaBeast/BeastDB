"use client";
import { Copy, Edit2, Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { RecordValue } from "./RecordValue";
import { UserProfilePanel } from "./UserProfilePanel";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: () => void;
  onNotice: (message: string, error?: boolean) => void;
}

function KeyInfoSection({ record, onNotice }: { record: UniversalRecord; onNotice: (m: string, e?: boolean) => void }) {
  const bits = BigInt(record.keyStr).toString(2).padStart(64, "0");
  const copy = async (text: string, label: string) => {
    try { await navigator.clipboard.writeText(text); onNotice(`${label} copied.`); }
    catch { onNotice("Copy failed.", true); }
  };
  return (
    <details className="group rounded-md border">
      <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">Key info</summary>
      <div className="space-y-3 border-t px-4 pb-4 pt-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <p className="text-xs text-muted-foreground">Decimal</p>
            <code className="break-all text-sm">{record.keyStr}</code>
          </div>
          <Button variant="ghost" size="icon" aria-label="Copy key" onClick={() => void copy(record.keyStr, "Key")}>
            <Copy size={14} />
          </Button>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">Hexadecimal</p>
          <code className="block break-all text-sm">{record.keyHex}</code>
        </div>
        <dl className="space-y-1 text-sm">
          <div className="flex justify-between"><dt className="text-muted-foreground">Partition</dt><dd>{record.prefixLabel}</dd></div>
          {record.userHash != null && <div className="flex justify-between"><dt className="text-muted-foreground">User hash</dt><dd className="font-mono">{record.userHash}</dd></div>}
          {record.itemHash != null && <div className="flex justify-between"><dt className="text-muted-foreground">Item hash</dt><dd className="font-mono">{record.itemHash}</dd></div>}
        </dl>
        <p className="text-xs text-muted-foreground">64-bit: {bits.slice(0, 8)} · {bits.slice(8, 36)} · {bits.slice(36)}</p>
      </div>
    </details>
  );
}

export function RecordDetails({ record, canWrite, onDelete, onDeleteUser, onEdit, onNotice }: Props) {
  // User profile: special layout
  if (record.prefix === 0x01) {
    return (
      <div className="flex-1 overflow-y-auto">
        <UserProfilePanel
          record={record}
          canWrite={canWrite}
          onDelete={onDelete}
          onDeleteUser={onDeleteUser ?? (() => {})}
        />
        <div className="px-5 pb-5">
          <KeyInfoSection record={record} onNotice={onNotice} />
        </div>
      </div>
    );
  }

  const formatted = (() => { try { return JSON.stringify(JSON.parse(record.raw), null, 2); } catch { return record.raw; } })();

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-5 py-5">
      {/* Cover image */}
      {record.coverUrl && (
        <img src={record.coverUrl} alt="" className="w-full max-h-40 rounded-lg object-cover" />
      )}
      {/* Fields */}
      {record.fields && (
        <section>
          <p className="mb-2 text-xs text-muted-foreground">{record.format.replace("_", " ")} · {formatBytes(record.byteSize)}</p>
          <div className="space-y-2">
            {Object.entries(record.fields).map(([k, v]) => (
              <div key={k} className="rounded-md border p-3">
                <div className="mb-1 font-mono text-xs font-medium text-muted-foreground">{k}</div>
                <RecordValue value={v} />
              </div>
            ))}
          </div>
        </section>
      )}
      {/* Array items */}
      {record.arrayItems && (
        <section>
          <p className="mb-2 text-xs text-muted-foreground">Array · {record.arrayItems.length} items</p>
          <div className="space-y-2">
            {record.arrayItems.map((v, i) => (
              <div key={i} className="rounded-md border p-3">
                <div className="mb-1 font-mono text-xs text-muted-foreground">[{i}]</div>
                <RecordValue value={v} />
              </div>
            ))}
          </div>
        </section>
      )}
      {/* Fallback */}
      {!record.fields && !record.arrayItems && (
        <div className="rounded-md border p-3 text-sm"><RecordValue value={record.raw || "Empty value"} /></div>
      )}
      {/* Raw JSON accordion */}
      <details className="rounded-md border">
        <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">Raw JSON</summary>
        <pre className="max-h-72 overflow-auto border-t bg-muted/50 px-4 py-3 text-xs leading-6 whitespace-pre-wrap break-all">{formatted}</pre>
      </details>
      {/* Key info accordion */}
      <KeyInfoSection record={record} onNotice={onNotice} />
      {/* Edit */}
      {canWrite && onEdit && (
        <Button variant="outline" size="sm" onClick={onEdit} className="self-start">
          <Edit2 size={14} /> Edit record
        </Button>
      )}
      {/* Delete */}
      {canWrite && (
        <Button variant="ghost" size="sm" className="self-start text-destructive" onClick={() => onDelete(record.keyStr)}>
          <Trash2 size={14} /> Delete record
        </Button>
      )}
    </div>
  );
}
