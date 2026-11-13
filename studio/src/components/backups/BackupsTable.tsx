"use client";
import { useCallback, useEffect, useState } from "react";
import { Check, Copy, Download, ShieldCheck } from "lucide-react";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { formatBytes } from "../../utils/format";

export interface BackupItem {
  id: string;
  created_at: string;
  file_size: number;
  kind?: string;
  created_by?: string;
  last_checkpoint_lsn: number;
  verified: { ok: boolean; at: string; error?: string } | null;
}

interface Listing {
  dir: string;
  intervalMinutes: number;
  keep: number;
  backups: BackupItem[];
}

/** Command that restores one server-made bundle. Restore stays an offline beastctl step. */
export const restoreCommand = (dir: string, id: string) =>
  `beastctl restore --backup ${dir}/${id} --dest ./restored`;

function VerifiedBadge({ v }: { v: BackupItem["verified"] }) {
  if (!v) return <Badge variant="outline" title="Not verified since the server started">Unverified</Badge>;
  return v.ok
    ? <Badge variant="lime" title={`Checked ${new Date(v.at).toLocaleString()}`}>Verified</Badge>
    : <Badge variant="mono" title={v.error}>FAILED</Badge>;
}

/** Server-made backups: list, create now, verify, download, copy the restore command. */
export function BackupsTable({
  canWrite, onNotice, reloadKey,
}: {
  canWrite: boolean;
  onNotice: (message: string, error?: boolean) => void;
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
      setData((await res.json()) as Listing);
      setState("ok");
    } catch {
      setState("error");
    }
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

  const copyRestore = async (id: string) => {
    try {
      await navigator.clipboard.writeText(restoreCommand(data?.dir ?? "./backups", id));
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
          {data && data.intervalMinutes > 0
            ? ` Automatic: every ${data.intervalMinutes} min, keeping the newest ${data.keep} scheduled.`
            : " Automatic backups are off."}
        </CardDescription>
      </CardHeader>
      <CardContent className="overflow-x-auto p-0">
        <table className="w-full min-w-[720px] text-sm">
          <thead>
            <tr className="border-y border-border text-left text-[11px] uppercase tracking-wide text-muted-foreground">
              <th scope="col" className="px-5 py-2.5 font-medium">Backup</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Created</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Size</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Type</th>
              <th scope="col" className="px-3 py-2.5 font-medium">Status</th>
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
                    <Button size="sm" variant="ghost" onClick={() => void copyRestore(b.id)} aria-label={`Copy restore command for ${b.id}`} title="Restore runs offline with beastctl">
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
