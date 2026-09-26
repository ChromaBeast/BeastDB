"use client";
import { Trash2, UserX } from "lucide-react";
import { UniversalRecord } from "../types";
import { RoleBadge } from "./RoleBadge";
import { Button } from "./ui/button";

interface Props {
  record: UniversalRecord;
  canWrite: boolean;
  onDelete: (key: string) => void;
  onDeleteUser: (userHash: number) => void;
}

export function UserProfilePanel({ record, canWrite, onDelete, onDeleteUser }: Props) {
  const name = record.fields?.username || record.fields?.name || "?";
  const initials = String(name).slice(0, 2).toUpperCase();
  const email = record.fields?.email as string | undefined;
  const role = record.fields?.role as string | undefined;
  const createdAt = record.fields?.createdAt as string | undefined;

  return (
    <div className="flex flex-col gap-4 p-5">
      {/* Avatar & Identifiers */}
      <div className="flex items-center gap-4">
        <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl bg-beast-lime text-xl font-bold font-mono text-black shadow-[0_0_20px_rgba(168,242,26,0.25)]">
          {initials}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-lg font-bold text-white tracking-tight">
              {record.fields?.username || record.primaryLabel}
            </h2>
            <RoleBadge role={role} />
          </div>
          {email && <p className="truncate text-xs text-zinc-400 font-mono mt-0.5">{email}</p>}
          <p className="mt-1 font-mono text-[11px] text-zinc-500 truncate">Hash: {record.userHash ?? "—"}</p>
        </div>
      </div>

      {/* Account Metadata Sheet */}
      <dl className="divide-y divide-zinc-800/80 rounded-lg border border-border bg-card text-xs">
        {createdAt && (
          <div className="flex justify-between gap-4 px-4 py-2.5">
            <dt className="text-zinc-400">Created</dt>
            <dd className="font-mono text-white">{new Date(createdAt).toLocaleString()}</dd>
          </div>
        )}
        <div className="flex justify-between gap-4 px-4 py-2.5">
          <dt className="text-zinc-400">Partition</dt>
          <dd className="font-mono text-beast-lime">0x01 (User Account)</dd>
        </div>
        <div className="flex justify-between gap-4 px-4 py-2.5">
          <dt className="text-zinc-400">Domain Hash (FNV-1a)</dt>
          <dd className="font-mono text-cyan-300">{record.userHash ?? "—"}</dd>
        </div>
      </dl>

      {/* Admin Management Actions */}
      {canWrite && (
        <div className="flex flex-col gap-2 pt-2 border-t border-zinc-800/80">
          <Button variant="outline" size="sm" className="justify-start gap-2 text-xs border-zinc-800 hover:text-white" onClick={() => onDelete(record.keyStr)}>
            <Trash2 size={13} className="text-zinc-400" /> Delete Credential Only
          </Button>
          {record.userHash != null && (
            <Button variant="destructive" size="sm" className="justify-start gap-2 text-xs bg-destructive/15 border border-destructive/30 text-destructive hover:bg-destructive hover:text-white" onClick={() => onDeleteUser(record.userHash!)}>
              <UserX size={13} /> Cascade Purge User & All Records
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
