"use client";
import { useCallback, useEffect, useState } from "react";
import { Check, Copy, Download, ShieldCheck, UploadCloud } from "lucide-react";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { formatBytes } from "../../utils/format";
import { restoreNewCommand } from "../../utils/backups";

export interface BackupItem {
  id: string;
  created_at: string;
  file_size: number;
  kind?: string;
  created_by?: string;
  last_checkpoint_lsn: number;
  encrypted?: boolean;
  verified: { ok: boolean; at: string; error?: string } | null;
  /** Last off-host copy attempt since the server started; null if none was made. */
  offsite: { ok: boolean; at: string; error?: string } | null;
}

export interface Destination { kind: string; where: string; enabled: boolean }

export interface Listing {
  dir: string;
  intervalMinutes: number;
  keep: number;
  backups: BackupItem[];
  /** Off-host destination from the server's startup flags; null when none is configured. */
  destination: Destination | null;
}

function VerifiedBadge({ v }: { v: BackupItem["verified"] }) {
  if (!v) return <Badge variant="outline" title="Not verified since the server started">Unverified</Badge>;
  return v.ok
    ? <Badge variant="lime" title={`Checked ${new Date(v.at).toLocaleString()}`}>Verified</Badge>
    : <Badge variant="mono" title={v.error}>FAILED</Badge>;
}

function OffsiteBadge({ b, dest }: { b: BackupItem; dest: Destination | null }) {
  if (!dest) return <span className="text-xs text-muted-foreground">No destination</span>;
  if (b.offsite) {
    return b.offsite.ok
      ? <Badge variant="lime" title={`Copied ${new Date(b.offsite.at).toLocaleString()} to ${dest.where}`}>Copied</Badge>
      : <Badge variant="mono" title={b.offsite.error}>Copy FAILED</Badge>;
  }
  return <Badge variant="outline" title="No copy attempt since the server started">{dest.enabled ? "Not copied" : "Paused"}</Badge>;
}

/** Server-made backups: list, create now, verify, download, copy off-host, copy the restore command. */
export function BackupsTable({
  canWrite, onNotice, reloadKey, onData,
}: {
  canWrite: boolean;
  onNotice: (message: string, error?: boolean) => void;
  /** Called with each fresh listing (the restore planner and settings use it). */
  onData?: (listing: Listing) => void;
  /** Change to force a reload (e.g. after Back up now in the page header). */
  reloadKey: number;
}) {
  const [data, setData] = useState<Listing | null>(null);
  const [state, setState] = useState<"loading" | "ok" | "unavailable" | "error">("loading");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await fetch("/api/backups", { cache: "no-store" });
      if (res.status === 404) return setState("unavailable");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const listing = (await res.json()) as Listing;
      setData(listing);
      onData?.(listing);
      setState("ok");
    } catch {
      setState("error");
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => { if (canWrite) void load(); }, [load, canWrite, reloadKey]);

  if (!canWrite || state === "unavailable") return null;

  const verify = async (id: string) => {
    setBusyId(id);
    try {
      const res = await fetch(`/api/backups/verify?id=${encodeURIComponent(id)}`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()).trim());
      const r = (await res.json()) as { ok: boolean; error?: string };
      onNotice(r.ok ? `${id} verified: checksum and manifest match.` : `${id} FAILED verification: ${r.error}`, !r.ok);
      await load();
    } catch (e) {
      onNotice(`Verify failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusyId(null);
    }
  };

  const sendOffsite = async (id: string) => {
    setBusyId(id);
    try {
      const res = await fetch(`/api/backups/copy?id=${encodeURIComponent(id)}`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
      const r = (await res.json()) as { ok: boolean; error?: string };
      onNotice(r.ok ? `${id} copied to the off-host destination.` : `Copy of ${id} FAILED: ${r.error}`, !r.ok);
      await load();
    } catch (e) {
      onNotice(`Copy failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusyId(null);
    }
  };

  const copyRestore = async (b: BackupItem) => {
    const id = b.id;
    try {
      await navigator.clipboard.writeText(restoreNewCommand({ dir: data?.dir ?? "./backups", id, newDir: "./restored", encrypted: b.encrypted }));
      setCopied(id);
      window.setTimeout(() => setCopied(null), 1500);
    } catch {
      onNotice("Could not access the clipboard.", true);
    }
  };

  const rows = data?.backups ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Backups</CardTitle>
        <CardDescription>
          Stored in <span className="font-mono">{data?.dir ?? "…"}</span> on this host.
          {data?.destination
            ? ` Also copied to ${data.destination.kind === "dir" ? "directory" : "hook"} ${data.destination.where}${data.destination.enabled ? "" : " (paused)"}.`
            : " No off-host destination is configured."}
          {data && data.intervalMinutes > 0
            ? ` Automatic: every ${data.intervalMinutes} min, keeping the newest ${data.keep} scheduled.`
            : " Automatic backups are off."}
        </CardDescription>
      </CardHeader>
      <CardContent className="overflow-x-auto p-0">
        <table className="w-full min-w-[860px] text-sm">
          <thead>
            <tr className="border-y border-border text-left text-[11px] uppercase tracking-wide text-muted-foreground">
              <th scope="col" className="px-5 py-2.5 font-medium">Backup</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Created</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Size</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Type</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Status</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Off-host</th>
              <th scope="col" className="px-5 py-2.5"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((b) => (
              <tr key={b.id} className="border-b border-border last:border-0">
                <td className="px-5 py-3 font-mono text-xs">{b.id}</td>
                <td className="px-3 py-3 text-xs text-muted-foreground">{new Date(b.created_at).toLocaleString()}</td>
                <td className="px-3 py-3 font-mono text-xs">{formatBytes(b.file_size)}</td>
                <td className="px-3 py-3 text-xs text-muted-foreground" title={b.created_by ? `by ${b.created_by}` : undefined}>{b.kind ?? "manual"}</td>
                <td className="px-3 py-3"><VerifiedBadge v={b.verified} /></td>
                <td className="px-3 py-3"><OffsiteBadge b={b} dest={data?.destination ?? null} /></td>
                <td className="px-5 py-3">
                  <div className="flex justify-end gap-1">
                    <Button size="sm" variant="ghost" disabled={busyId === b.id} onClick={() => void verify(b.id)} aria-label={`Verify ${b.id}`}>
                      <ShieldCheck size={14} /> {busyId === b.id ? "Checking…" : "Verify"}
                    </Button>
                    <a
                      href={`/api/backups/download?id=${encodeURIComponent(b.id)}`}
                      download
                      aria-label={`Download ${b.id}`}
                      className="inline-flex h-8 items-center justify-center gap-2 rounded-md px-3 text-xs font-medium transition-colors hover:bg-muted hover:text-foreground"
                    >
                      <Download size={14} /> Download
                    </a>
                    {data?.destination && (
                      <Button size="sm" variant="ghost" disabled={busyId === b.id} onClick={() => void sendOffsite(b.id)} aria-label={`Copy ${b.id} to the off-host destination`}>
                        <UploadCloud size={14} /> Send off-host
                      </Button>
                    )}
                    <Button size="sm" variant="ghost" onClick={() => void copyRestore(b)} aria-label={`Copy restore command for ${b.id}`} title="Copies a beastctl command that restores into a new directory">
                      {copied === b.id ? <Check size={14} className="text-success" /> : <Copy size={14} />} Restore cmd
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {state === "loading" && <p className="p-5 text-sm text-muted-foreground">Loading backups…</p>}
        {state === "error" && <p className="p-5 text-sm text-destructive">Could not read the backup list.</p>}
        {state === "ok" && rows.length === 0 && <p className="p-5 text-sm text-muted-foreground">No backups yet. Use “Back up now”.</p>}
      </CardContent>
    </Card>
  );
}
