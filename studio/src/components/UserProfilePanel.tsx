"use client";
import { Loader2, Trash2, UserX } from "lucide-react";
import { UniversalRecord } from "../types";
import { RoleBadge } from "./RoleBadge";
import { Button } from "./ui/button";
import { UserCollectionSection } from "./UserCollectionSection";
import { useUserContext } from "../hooks/useUserContext";
import { formatDateRelative } from "../utils/format";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser: (userHash: number) => void;
  onScanRange: (start: string, end: string) => Promise<UniversalRecord[]>;
  onSelect: (record: UniversalRecord) => void;
}

export function UserProfilePanel({ record, canWrite, onDelete, onDeleteUser, onScanRange, onSelect }: Props) {
  const name = record.fields?.username || record.fields?.name || "?";
  const initials = String(name).slice(0, 2).toUpperCase();
  const email = record.fields?.email as string | undefined;
  const role = record.fields?.role as string | undefined;
  const createdAt = record.fields?.createdAt as string | undefined;

  const { collections, ready } = useUserContext(record.userHash, onScanRange);

  const totalRelated = collections.reduce((s, c) => s + c.records.length, 0);

  return (
    <div className="flex flex-col gap-4 p-5">
      {/* Avatar & Identifiers */}
      <div className="flex items-center gap-4">
        <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl bg-beast-lime text-xl font-bold font-mono text-black shadow-[0_0_20px_rgba(168,242,26,0.25)]">
          {initials}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-lg font-bold text-foreground tracking-tight">
              {record.fields?.username || record.primaryLabel}
            </h2>
            <RoleBadge role={role} />
          </div>
          {email && <p className="truncate text-xs text-muted-foreground font-mono mt-0.5">{email}</p>}
          {createdAt && <p className="text-[11px] text-muted-foreground mt-0.5">Joined {formatDateRelative(createdAt)}</p>}
        </div>
      </div>

      {/* Collection summary chips */}
      <div className="flex flex-wrap gap-1.5">
        {collections.map((c) => (
          <span key={c.prefix} className="rounded-full border border-border bg-muted px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
            {c.label} <span className="font-mono text-foreground">{c.records.length}</span>
          </span>
        ))}
        {!ready && (
          <span className="flex items-center gap-1 text-[11px] text-muted-foreground">
            <Loader2 size={10} className="animate-spin" /> Loading…
          </span>
        )}
        {ready && totalRelated === 0 && (
          <span className="text-[11px] text-muted-foreground">No related records found.</span>
        )}
      </div>

      {/* Cross-partition collections */}
      {collections.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Library</h3>
          {collections.map((c) => (
            <UserCollectionSection key={c.prefix} collection={c} onSelect={onSelect} />
          ))}
        </section>
      )}

      {/* Admin actions */}
      {canWrite && (
        <div className="flex flex-col gap-2 pt-2 border-t border-border">
          <Button variant="outline" size="sm" className="justify-start gap-2 text-xs" onClick={() => onDelete(record.keyStr)}>
            <Trash2 size={13} className="text-muted-foreground" /> Delete Credential Only
          </Button>
          {record.userHash != null && (
            <Button
              variant="destructive" size="sm"
              className="justify-start gap-2 text-xs bg-destructive/15 border border-destructive/30 text-destructive hover:bg-destructive hover:text-destructive-foreground"
              onClick={() => onDeleteUser(record.userHash!)}
            >
              <UserX size={13} /> Cascade Purge User & All Records
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
