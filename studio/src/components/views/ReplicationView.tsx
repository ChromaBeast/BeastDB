"use client";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "../PageHeader";
import { CopyCommand } from "../CopyCommand";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { PromoteCard } from "../replication/PromoteCard";
import { RejoinCard } from "../replication/RejoinCard";
import { formatAckAge, replicaState, replicaStateLabel, summarizeReplicas } from "../../utils/replicas";
import type { ReplicationStatus } from "../../types";
import type { ViewProps } from "./types";

const POLL_MS = 5000;

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-1 font-mono text-xl font-semibold ${tone ?? "text-foreground"}`}>{value}</p>
    </div>
  );
}

/** Replication: this node's role/LSN plus the leader's live replica table (admin-only endpoint). */
export function ReplicationView({ stats, user, onNotice }: ViewProps) {
  const [data, setData] = useState<ReplicationStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const isAdmin = user?.role === "admin";

  const load = useCallback(async () => {
    if (!isAdmin) return;
    try {
      const res = await fetch("/api/replication");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setData((await res.json()) as ReplicationStatus);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "request failed");
    }
  }, [isAdmin]);

  useEffect(() => {
    void load();
    if (!isAdmin) return;
    const t = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(t);
  }, [load, isAdmin]);

  const role = data?.role ?? stats?.role ?? "standalone";
  const lsn = data?.lsn ?? stats?.lsn ?? 0;
  const replicas = data?.replicas ?? [];
  const summary = summarizeReplicas(replicas);
  const connected = summary.connected;
  const isLeader = role === "leader";

  return (
    <section aria-labelledby="replication-title" className="space-y-6">
      <PageHeader
        id="replication-title"
        title="Replication"
        subtitle="Leader/follower WAL streaming. Only data the engine tracks is shown."
        actions={
          isAdmin && (
            <Button variant="outline" size="sm" onClick={() => void load()}>
              Refresh
            </Button>
          )
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="This node" value={role} tone={isLeader ? "text-primary" : undefined} />
        <Stat label="Current LSN" value={lsn.toLocaleString()} />
        <Stat
          label="Replicas connected"
          value={data?.tracked ? String(connected) : "n/a"}
          tone={data?.tracked && connected > 0 ? "text-success" : "text-muted-foreground"}
        />
      </div>

      {isAdmin && data?.cluster && (
        <PromoteCard cluster={data.cluster} isAdmin={isAdmin} onNotice={onNotice} onPromoted={() => void load()} />
      )}

      {isAdmin && data?.cluster?.role === "fenced" && (
        <RejoinCard cluster={data.cluster} onNotice={onNotice} onRejoined={() => void load()} />
      )}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-sm">Replicas</CardTitle>
          {data?.tracked && <Badge variant="mono">{replicas.length} seen since start</Badge>}
        </CardHeader>
        <CardContent>
          {!isAdmin ? (
            <p className="text-sm text-muted-foreground">Replica details are visible to admin users only.</p>
          ) : error ? (
            <p className="text-sm text-destructive">Could not load replication status: {error}</p>
          ) : !data ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : !data.tracked ? (
            <p className="text-sm text-muted-foreground">
              Replicas are not tracked on this node. Only a leader sees its followers; open Studio on the leader to
              view them.
            </p>
          ) : replicas.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No replicas connected. A follower appears here once it opens a WAL stream to this leader.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-muted-foreground">
                  <tr className="border-b border-border">
                    <th className="py-2 pr-4 font-medium">Replica</th>
                    <th className="py-2 pr-4 font-medium">Address</th>
                    <th className="py-2 pr-4 font-medium">State</th>
                    <th className="py-2 pr-4 text-right font-medium">Acked LSN</th>
                    <th className="py-2 pr-4 text-right font-medium">Lag (LSN)</th>
                    <th className="py-2 text-right font-medium">Last ack</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {replicas.map((r) => (
                    <tr key={r.id}>
                      <td className="py-2.5 pr-4 font-mono text-foreground">{r.id}</td>
                      <td className="py-2.5 pr-4 font-mono text-muted-foreground">{r.address || "—"}</td>
                      <td className="py-2.5 pr-4">
                        <Badge variant={replicaState(r) === "streaming" ? "lime" : "outline"}>{replicaStateLabel(replicaState(r))}</Badge>
                      </td>
                      <td className="py-2.5 pr-4 text-right font-mono">{r.ackedLSN.toLocaleString()}</td>
                      <td className={`py-2.5 pr-4 text-right font-mono ${r.lagLSN > 0 ? "text-warning" : "text-foreground"}`}>
                        {r.lagLSN.toLocaleString()}
                      </td>
                      <td className="py-2.5 text-right font-mono text-muted-foreground">{formatAckAge(r.lastAckAgoMS)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {data?.tracked && replicas.length > 0 && (
            <div className="mt-3 space-y-1.5 text-xs text-muted-foreground">
              <p data-testid="replica-summary">
                {summary.connected} connected, {summary.lagging} behind or silent, {summary.disconnected} disconnected.
              </p>
              <p>
                Lag is the leader&apos;s current LSN minus the replica&apos;s last acknowledged LSN, so it is only as fresh
                as the last ack (followers ack at least once a second while connected). A disconnected replica keeps its
                last ack until the leader restarts. To promote a replica, open Studio on that replica: promotion runs on
                the node being promoted.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Add a replica</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {!isAdmin ? (
            <p className="text-sm text-muted-foreground">Visible to admin users only.</p>
          ) : !data?.join ? (
            <p className="text-sm text-muted-foreground">
              This node does not report how to join a follower to it.
            </p>
          ) : !data.join.available ? (
            <p className="text-sm text-muted-foreground" data-testid="join-unavailable">
              {data.join.reason}
            </p>
          ) : (
            <>
              <CopyCommand label="Run on the new follower's machine" command={data.join.command ?? ""} onNotice={onNotice} />
              <ul className="list-disc space-y-1 pl-5 text-xs text-muted-foreground" data-testid="join-notes">
                {data.join.notes?.map((n) => (
                  <li key={n}>{n}</li>
                ))}
              </ul>
            </>
          )}
          <p className="text-xs text-muted-foreground">
            Studio cannot start or register a replica: there is no join protocol. A follower is a process started with
            these flags; the leader sees it when it opens a WAL stream. It runs read-only and replays the leader&apos;s
            WAL from the start. Promotion is manual and failover is not automatic.
          </p>
        </CardContent>
      </Card>
    </section>
  );
}
