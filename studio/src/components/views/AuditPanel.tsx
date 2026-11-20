"use client";
import { useCallback, useEffect, useState, type ReactNode } from "react";
import { PageHeader } from "../PageHeader";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card } from "../ui/card";

export interface AuditEntry {
  seq: number;
  time: string;
  actor: string;
  action: string;
  target?: string;
  detail?: string;
  ip?: string;
}

interface AuditStatus {
  entries: number;
  verified: boolean;
  error?: string;
}

export const AUDIT_ACTIONS = [
  "login.ok", "login.failed", "logout", "token.create", "token.revoke",
  "user.delete", "record.put", "record.delete", "checkpoint", "settings.update",
  "backup.create", "backup.verify", "backup.download",
] as const;

const SELECT = "h-8 rounded-md border border-border bg-card px-2 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
const PAGE = 100;

const TONE: Record<string, string> = {
  "login.failed": "text-warning",
  "user.delete": "text-destructive",
  "record.delete": "text-destructive",
  "token.revoke": "text-destructive",
};

/** Admin actions from the tamper-evident audit log (GET /api/audit). */
export function AuditPanel({ tabs }: { tabs: ReactNode }) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [status, setStatus] = useState<AuditStatus | null>(null);
  const [action, setAction] = useState("");
  const [more, setMore] = useState(false);
  const [state, setState] = useState<"loading" | "ok" | "error">("loading");

  const load = useCallback(async (before?: number) => {
    try {
      const qs = new URLSearchParams({ limit: String(PAGE) });
      if (action) qs.set("action", action);
      if (before) qs.set("before", String(before));
      const res = await fetch(`/api/audit?${qs}`, { cache: "no-store" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const body = (await res.json()) as { entries: AuditEntry[]; status: AuditStatus };
      setEntries((prev) => (before ? [...prev, ...body.entries] : body.entries));
      setMore(body.entries.length === PAGE);
      setStatus(body.status);
      setState("ok");
    } catch {
      setState("error");
    }
  }, [action]);

  useEffect(() => {
    setState("loading");
    void load();
  }, [load]);

  return (
    <section aria-labelledby="logs-title" className="space-y-4">
      <PageHeader
        id="logs-title"
        title="Logs & Audit"
        subtitle="Sign-ins, key changes, deletes and checkpoints. Append-only and hash-chained; record values and secrets are never written."
        actions={
          status && (
            <Badge variant={status.verified ? "lime" : "mono"} title={status.error}>
              {status.verified ? `CHAIN VERIFIED · ${status.entries}` : "CHAIN BROKEN"}
            </Badge>
          )
        }
      />
      <div className="flex flex-wrap items-center gap-2">
        {tabs}
        <select value={action} onChange={(e) => setAction(e.target.value)} aria-label="Filter by action" className={`${SELECT} ml-auto`}>
          <option value="">All actions</option>
          {AUDIT_ACTIONS.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
      </div>

      {status && !status.verified && (
        <p role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          The audit chain does not verify: {status.error}. The log was edited, truncated or reordered after it was written.
        </p>
      )}

      <Card className="overflow-x-auto">
        <table className="w-full min-w-[720px] text-sm">
          <thead>
            <tr className="border-b border-border text-left text-[11px] uppercase tracking-wide text-muted-foreground">
              <th scope="col" className="px-4 py-3 font-medium">Time</th>
              <th scope="col" className="px-3 py-3 font-medium">Actor</th>
              <th scope="col" className="px-3 py-3 font-medium">Action</th>
              <th scope="col" className="px-3 py-3 font-medium">Target</th>
              <th scope="col" className="px-4 py-3 font-medium">Detail</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.seq} className="border-b border-border last:border-0">
                <td className="whitespace-nowrap px-4 py-2.5 font-mono text-xs text-muted-foreground" title={e.ip ? `from ${e.ip}` : undefined}>
                  {new Date(e.time).toLocaleString()}
                </td>
                <td className="px-3 py-2.5 font-mono text-xs">{e.actor}</td>
                <td className={`px-3 py-2.5 font-mono text-xs ${TONE[e.action] ?? "text-foreground"}`}>{e.action}</td>
                <td className="px-3 py-2.5 font-mono text-xs text-muted-foreground">{e.target || "—"}</td>
                <td className="px-4 py-2.5 text-xs text-muted-foreground">{e.detail || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {state === "loading" && <p className="p-4 text-sm text-muted-foreground">Loading audit log…</p>}
        {state === "error" && <p className="p-4 text-sm text-destructive">Could not read the audit log.</p>}
        {state === "ok" && entries.length === 0 && <p className="p-4 text-sm text-muted-foreground">No matching entries yet.</p>}
      </Card>
      {more && (
        <div className="flex justify-center">
          <Button variant="outline" size="sm" onClick={() => void load(entries[entries.length - 1]?.seq)}>Load older</Button>
        </div>
      )}
    </section>
  );
}
