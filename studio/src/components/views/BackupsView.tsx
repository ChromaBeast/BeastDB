"use client";
import { useState } from "react";
import { PageHeader } from "../PageHeader";
import { StatCard, StatGrid } from "../StatCard";
import { BackupsTable, type Listing } from "../backups/BackupsTable";
import { RestorePlanner } from "../backups/RestorePlanner";
import { RuntimeSettingsCard } from "../settings/RuntimeSettings";
import { CopyCommand } from "../CopyCommand";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Button } from "../ui/button";
import { formatBytes } from "../../utils/format";
import type { ViewProps } from "./types";

// Commands mirror the real flags in cmd/beastctl/main.go.
const BACKUP_CMD = "beastctl backup --db ./data/beast.bin --wal ./data/beast.wal --out ./backup";
const VERIFY_CMD = "beastctl verify --backup ./backup";
const RESTORE_CMD = "beastctl restore --backup ./backup --new-dir ./restored";

const CHECKLIST = [
  "Prefer --new-dir: it needs a path that does not exist, never overwrites, verifies first and installs only a database that opened cleanly.",
  "Run verify on the bundle before relying on it; restore also verifies first. An encrypted bundle needs --key-file to authenticate its pages.",
  "Check the record count it prints, then start a server with -data-dir on the new directory and confirm before cutting over.",
  "--dest --force overwrites an existing directory. Use it only with the server stopped; beastctl opens the data files directly and there is no file lock.",
  "A backup holds the writes committed before its checkpoint. Writes made while it was copying are not in it.",
  "Keep at least one copy off the host: set -backup-copy-dir or -backup-copy-exec when starting the server. Retention applies to the local directory only.",
];

function Row({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="flex items-center justify-between py-2.5 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className={`font-mono ${tone ?? "text-foreground"}`}>{value}</span>
    </div>
  );
}

function lastCheckpoint(unix?: number): string {
  if (!unix) return "none since this process started";
  return new Date(unix * 1000).toLocaleString();
}

/** Backups: durability state, a real checkpoint action, and the offline beastctl workflow. */
export function BackupsView({ stats, user, onNotice, onRefresh }: ViewProps) {
  const [checkpointing, setCheckpointing] = useState(false);
  const [backingUp, setBackingUp] = useState(false);
  const [reload, setReload] = useState(0);
  const [listing, setListing] = useState<Listing | null>(null);
  const s = stats?.storage;
  const dirty = s?.dirtyPages ?? 0;
  const isAdmin = user?.role === "admin";

  const checkpoint = async () => {
    setCheckpointing(true);
    try {
      const res = await fetch("/api/checkpoint", { method: "POST" });
      if (!res.ok) {
        const detail = (await res.text()).trim();
        throw new Error(detail || `HTTP ${res.status}`);
      }
      onNotice("Checkpoint complete: dirty pages flushed and the WAL rotated.");
      onRefresh();
    } catch (e) {
      onNotice(`Checkpoint failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setCheckpointing(false);
    }
  };

  const backUpNow = async () => {
    setBackingUp(true);
    try {
      const res = await fetch("/api/backups", { method: "POST" });
      if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
      const body = (await res.json()) as { backup: { id: string }; tookMs: number };
      onNotice(`Backup ${body.backup.id} created in ${(body.tookMs / 1000).toFixed(1)} s and verified.`);
      setReload((n) => n + 1);
    } catch (e) {
      onNotice(`Backup failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBackingUp(false);
    }
  };

  return (
    <section aria-labelledby="backups-title" className="space-y-6">
      <PageHeader
        id="backups-title"
        title="Backups"
        subtitle="Create, verify, download and copy off-host backups of this node. Restoring is a beastctl command into a new directory."
        actions={
          isAdmin ? (
            <Button size="sm" onClick={() => void backUpNow()} disabled={backingUp} title="Reads and writes pause only for a checkpoint, not for the copy">
              {backingUp ? "Backing up…" : "Back up now"}
            </Button>
          ) : undefined
        }
      />

      <StatGrid>
        <StatCard
          label="Last checkpoint"
          value={s?.lastCheckpointUnix ? new Date(s.lastCheckpointUnix * 1000).toLocaleTimeString() : "None yet"}
          hint={s?.lastCheckpointUnix ? `LSN ${(s.lastCheckpointLSN ?? 0).toLocaleString()}` : "since this process started"}
          tone={s?.lastCheckpointUnix ? "success" : "muted"}
        />
        <StatCard label="WAL size" value={formatBytes(s?.walSizeBytes ?? 0)} hint={`LSN ${(s?.currentLSN ?? stats?.lsn ?? 0).toLocaleString()}`} />
        <StatCard label="Dirty pages" value={`${dirty}`} hint={`of ${s?.poolSize ?? 0} frames`} tone={dirty > 0 ? "warning" : "muted"} />
        <StatCard label="Data file" value={formatBytes(s?.diskSizeBytes ?? 0)} hint={`${s?.totalPages ?? 0} pages`} />
      </StatGrid>

      <BackupsTable canWrite={isAdmin} onNotice={onNotice} reloadKey={reload} onData={setListing} />

      {isAdmin && (
        <RuntimeSettingsCard
          key={listing?.destination ? "with-destination" : "local-only"}
          title="Automatic backups"
          description="Off by default. Each backup pauses reads and writes only for a checkpoint, but the copy competes for disk and delays automatic checkpoints until it ends, so choose a quiet interval. Manual backups are never pruned."
          onNotice={(m, e) => { onNotice(m, e); setReload((n) => n + 1); }}
          fields={[
            { name: "backup_interval_minutes", label: "Back up every", unit: "minutes (0 = off, otherwise 5 or more)" },
            { name: "backup_keep", label: "Keep the newest", unit: "scheduled backups" },
            ...(listing?.destination
              ? [{ name: "backup_offsite", label: "Copy new backups off-host", unit: "1 = copy, 0 = pause" }]
              : []),
          ]}
        />
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-sm">Durability</CardTitle>
            {isAdmin && (
              <Button size="sm" onClick={() => void checkpoint()} disabled={checkpointing}>
                {checkpointing ? "Checkpointing…" : "Checkpoint now"}
              </Button>
            )}
          </CardHeader>
          <CardContent className="divide-y divide-border/60">
            <Row label="Current LSN" value={(s?.currentLSN ?? stats?.lsn ?? 0).toLocaleString()} />
            <Row label="WAL size" value={formatBytes(s?.walSizeBytes ?? 0)} />
            <Row label="Dirty pages" value={`${dirty} of ${s?.poolSize ?? 0}`} tone={dirty > 0 ? "text-warning" : undefined} />
            <Row label="Last checkpoint LSN" value={(s?.lastCheckpointLSN ?? 0).toLocaleString()} />
            <Row label="Last checkpoint" value={lastCheckpoint(s?.lastCheckpointUnix)} />
            <Row label="Data file" value={`${formatBytes(s?.diskSizeBytes ?? 0)} (${s?.totalPages ?? 0} pages)`} />
          </CardContent>
          <CardContent className="pt-0">
            <p className="text-xs text-muted-foreground">
              A checkpoint flushes dirty pages, syncs the data file, then rotates the WAL. The server also checkpoints
              automatically when the WAL passes <span className="font-mono">-wal-checkpoint-mb</span>.
              {!isAdmin && " Only admin users can trigger one."}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">How to back up</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <CopyCommand label="1. Create a bundle (data.bin + manifest.json with SHA-256)" command={BACKUP_CMD} onNotice={onNotice} />
            <CopyCommand label="2. Verify the manifest and checksum" command={VERIFY_CMD} onNotice={onNotice} />
            <CopyCommand label="3. Restore into a new directory (never overwrites)" command={RESTORE_CMD} onNotice={onNotice} />
            <p className="text-xs text-muted-foreground">
              Step 1 needs the server stopped. Backups made by this server (above) need no CLI step.
              Defaults shown match the CLI.
            </p>
          </CardContent>
        </Card>
      </div>

      {isAdmin && <RestorePlanner listing={listing} onNotice={onNotice} />}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Restore safety checklist</CardTitle>
          </CardHeader>
          <CardContent>
            <ol className="list-decimal space-y-2 pl-5 text-xs text-muted-foreground marker:font-mono">
              {CHECKLIST.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ol>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Not provided</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-xs text-muted-foreground">
            <p><span className="text-foreground">Point-in-time recovery.</span> A backup is a checkpoint image, not a WAL archive. Planned, not built.</p>
            <p><span className="text-foreground">Restore over the running database.</span> By design there is no button or endpoint for it.</p>
            <p><span className="text-foreground">Built-in cloud upload.</span> Off-host copy is a directory or a program you supply (the program holds any credentials).</p>
            <p><span className="text-foreground">Pruning off-host copies.</span> Retention applies to the local directory only.</p>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
