"use client";
import { Edit2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { Button } from "./ui/button";
import { RecordValue } from "./RecordValue";
import { RecordFieldList } from "./RecordFieldList";
import { UserProfilePanel } from "./UserProfilePanel";
import { RecordEngineDetails } from "./RecordEngineDetails";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser?: (userHash: number) => void;
  onEdit?: () => void;
  onNotice: (message: string, error?: boolean) => void;
  onScanRange?: (start: string, end: string) => Promise<UniversalRecord[]>;
  onSelect?: (record: UniversalRecord) => void;
  partitionCounts?: Record<string, number>;
}

export function RecordDetails({
  record, canWrite, onDelete, onDeleteUser, onEdit, onNotice, onScanRange, onSelect, partitionCounts,
}: Props) {
  const formatted = (() => {
    try {
      return JSON.stringify(JSON.parse(record.raw), null, 2);
    } catch {
      return record.raw;
    }
  })();

  const noop = async () => [] as UniversalRecord[];
  const isUserRecord = Boolean(record.fields?.email || record.fields?.username);

  if (isUserRecord) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <UserProfilePanel
          record={record}
          canWrite={canWrite}
          onDelete={onDelete}
          onDeleteUser={onDeleteUser ?? (() => {})}
          onScanRange={onScanRange ?? noop}
          onSelect={onSelect ?? (() => {})}
          partitionCounts={partitionCounts}
        />
        <div className="px-5 pb-5">
          <RecordEngineDetails
            record={record}
            formattedRaw={formatted}
            canWrite={canWrite}
            onDelete={onDelete}
            onNotice={onNotice}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-5">
      {/* Content Header */}
      <div className="border-b border-border/70 pb-4">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate text-xs font-medium text-muted-foreground">{record.prefixLabel}</span>
          {canWrite && onEdit && (
            <Button variant="outline" size="sm" onClick={onEdit} className="h-7 shrink-0 gap-1 text-xs">
              <Edit2 size={13} /> Edit
            </Button>
          )}
        </div>
        <h2 className="mt-2 text-lg font-bold tracking-tight text-foreground">{record.primaryLabel}</h2>
        {record.secondaryLabel && <p className="text-xs text-muted-foreground mt-0.5">{record.secondaryLabel}</p>}
      </div>

      {/* Cover Image Preview */}
      {record.coverUrl && (
        <img src={record.coverUrl} alt="" className="w-full max-h-56 rounded-xl object-cover border border-border shadow-sm" />
      )}

      {/* Structured Fields */}
      {record.fields && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Document Fields</h3>
          <RecordFieldList fields={record.fields} />
        </section>
      )}

      {/* Array Items */}
      {record.arrayItems && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Items ({record.arrayItems.length})</h3>
          <div className="rounded-lg border border-border bg-card p-3 text-sm">
            <RecordValue value={record.arrayItems} />
          </div>
        </section>
      )}

      {/* Raw Fallback */}
      {!record.fields && !record.arrayItems && (
        <div className="rounded-lg border border-border bg-card p-3 text-sm">
          <RecordValue value={record.raw || "Empty value"} />
        </div>
      )}

      {/* Engine & Storage Internals (Collapsible) */}
      <RecordEngineDetails
        record={record}
        formattedRaw={formatted}
        canWrite={canWrite}
        onDelete={onDelete}
        onNotice={onNotice}
      />
    </div>
  );
}
