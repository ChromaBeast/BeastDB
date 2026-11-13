"use client";
import { useState } from "react";
import { PageHeader } from "../PageHeader";
import { CopyCommand } from "../CopyCommand";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { formatBytes } from "../../utils/format";
import type { ViewProps } from "./types";

// Commands mirror the real flags in cmd/beastctl/main.go.
const BACKUP_CMD = "beastctl backup --db ./data/beast.bin --wal ./data/beast.wal --out ./backup";
const VERIFY_CMD = "beastctl verify --backup ./backup";
const RESTORE_CMD = "beastctl restore --backup ./backup --dest ./restored";

const CHECKLIST = [
  "Stop the node (or back up a copy of its data directory). beastctl opens the data files directly and there is no file lock.",
  "Run verify on the bundle before relying on it; restore also verifies first.",
  "Restore into an empty directory. Without --force it refuses to overwrite an existing beast.bin.",
  "Restore removes any old beast.wal at the destination and opens the result once to count records.",
  "Point the server at the restored directory with -data-dir and confirm the record count before cutting over.",
  "Keep at least one copy off the host. Retention and off-host copy are not managed by BeastDB.",
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

  return (
    <section aria-labelledby="backups-title" className="space-y-6">
      <PageHeader
        id="backups-title"
        title="Backups"
        subtitle="Backups are produced offline with beastctl. Studio shows durability state and runs checkpoints."
      />

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
            <CopyCommand label="3. Restore into a destination directory" command={RESTORE_CMD} onNotice={onNotice} />
            <p className="text-xs text-muted-foreground">
              Add <span className="font-mono">--force</span> to restore only when you intend to overwrite an existing
              database. Defaults shown match the CLI.
            </p>
          </CardContent>
        </Card>
      </div>

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
            <CardTitle className="text-sm">Not available yet</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-xs">
            {["Scheduled / automatic backups", "Point-in-time recovery (PITR)", "Retention policies and off-host copy"].map((name) => (
              <div key={name} className="flex items-center justify-between">
                <span className="text-foreground">{name}</span>
                <Badge variant="outline">Planned</Badge>
              </div>
            ))}
            <p className="text-muted-foreground">
              These are on the roadmap (ROADMAP.md) and have no controls here. Until then, run beastctl from your own
              scheduler and copy bundles off the host yourself.
            </p>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
