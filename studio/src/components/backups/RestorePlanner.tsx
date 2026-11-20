"use client";
import { useState } from "react";
import { CopyCommand } from "../CopyCommand";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { formatBytes } from "../../utils/format";
import { restoreNewCommand, restoreReadiness } from "../../utils/backups";
import type { Listing } from "./BackupsTable";

const TONE = { ok: "text-success", warn: "text-warning", bad: "text-destructive" } as const;

/**
 * Builds the `beastctl restore --new-dir` command for a chosen backup and shows
 * whether it is ready to restore. It runs nothing: restoring happens offline in
 * a terminal, into a directory that must not exist yet, so it cannot touch the
 * live database.
 */
export function RestorePlanner({ listing, onNotice }: { listing: Listing | null; onNotice: (m: string, e?: boolean) => void }) {
  const [picked, setPicked] = useState("");
  const [newDir, setNewDir] = useState("./restored");
  const [keyFile, setKeyFile] = useState("");
  const backups = listing?.backups ?? [];
  const b = backups.find((x) => x.id === picked) ?? backups[0];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Restore into a new database</CardTitle>
        <CardDescription>
          Generates a command to run in a terminal. It restores into a directory that does not exist yet and never
          overwrites one. There is no restore button: Studio does not change the running database.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {!b ? (
          <p className="text-xs text-muted-foreground">No backups yet. Create one first.</p>
        ) : (
          <>
            <label className="block text-xs">
              <span className="mb-1 block font-medium text-muted-foreground">Backup</span>
              <select
                className="h-9 w-full rounded-md border border-border bg-background px-2 font-mono text-xs"
                value={b.id}
                onChange={(e) => setPicked(e.target.value)}
              >
                {backups.map((x) => <option key={x.id} value={x.id}>{x.id}</option>)}
              </select>
            </label>
            <label className="block text-xs">
              <span className="mb-1 block font-medium text-muted-foreground">New data directory (must not exist)</span>
              <Input value={newDir} onChange={(e) => setNewDir(e.target.value)} className="font-mono text-xs" />
            </label>
            {b.encrypted && (
              <label className="block text-xs">
                <span className="mb-1 block font-medium text-muted-foreground">Master key file path on the machine that restores</span>
                <Input value={keyFile} onChange={(e) => setKeyFile(e.target.value)} placeholder="/path/to/master.key" className="font-mono text-xs" />
              </label>
            )}
            <ul className="divide-y divide-border/60 text-xs" aria-label="Restore readiness">
              {restoreReadiness(b, formatBytes).map((r) => (
                <li key={r.label} className="flex gap-3 py-2">
                  <span className={`w-24 shrink-0 font-medium ${TONE[r.state]}`}>{r.label}</span>
                  <span className="text-muted-foreground">{r.text}</span>
                </li>
              ))}
            </ul>
            <CopyCommand
              label="Run this on a host that can read the bundle"
              command={restoreNewCommand({ dir: listing?.dir ?? "./backups", id: b.id, newDir, encrypted: b.encrypted, keyFile })}
              onNotice={onNotice}
            />
            <p className="text-xs text-muted-foreground">
              It verifies the bundle, builds and opens the database in a staging directory beside the target, and moves
              it into place only if that worked. Then start a server with <span className="font-mono">-data-dir</span> on
              the new directory.
            </p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
