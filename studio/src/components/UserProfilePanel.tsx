"use client";
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
  const initials = name.slice(0, 2).toUpperCase();
  const email = record.fields?.email as string | undefined;
  const role = record.fields?.role as string | undefined;
  const createdAt = record.fields?.createdAt as string | undefined;

  return (
    <div className="flex flex-col gap-4 p-5">
      {/* Avatar */}
      <div className="flex items-center gap-4">
        <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-full bg-beast-lime text-xl font-bold text-black shadow-[0_0_15px_rgba(168,242,26,0.2)]">
          {initials}
        </div>
        <div className="min-w-0">
          <h2 className="truncate text-lg font-semibold">
            {record.fields?.username || record.primaryLabel}
          </h2>
          {email && <p className="truncate text-sm text-muted-foreground">{email}</p>}
          <div className="mt-1">
            <RoleBadge role={role} />
          </div>
        </div>
      </div>
      {/* Details */}
      <dl className="divide-y rounded-md border text-sm">
        {createdAt && (
          <div className="flex justify-between gap-4 px-4 py-2.5">
            <dt className="text-muted-foreground">Created</dt>
            <dd>{new Date(createdAt).toLocaleDateString()}</dd>
          </div>
        )}
        <div className="flex justify-between gap-4 px-4 py-2.5">
          <dt className="text-muted-foreground">User hash</dt>
          <dd className="font-mono text-xs">{record.userHash ?? "—"}</dd>
        </div>
      </dl>
      {/* Actions */}
      {canWrite && (
        <>
          <hr />
          <div className="flex flex-col gap-2">
            <Button variant="outline" className="border-destructive text-destructive hover:bg-destructive/10" onClick={() => onDelete(record.keyStr)}>
              Delete this record
            </Button>
            {record.userHash != null && (
              <Button variant="destructive" onClick={() => onDeleteUser(record.userHash!)}>
                Delete user + all data
              </Button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
