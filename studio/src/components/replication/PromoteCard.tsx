"use client";
import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from "../ui/alert-dialog";
import { promoteErrorMessage, promoteGate } from "../../utils/promote";
import type { ClusterStatus } from "../../types";

interface Props {
  cluster: ClusterStatus;
  isAdmin: boolean;
  onNotice: (message: string, error?: boolean) => void;
  onPromoted: () => void;
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono text-foreground">{value}</span>
    </div>
  );
}

/** This node's epoch and a manual Promote action, enabled only when the node is caught up. */
export function PromoteCard({ cluster, isAdmin, onNotice, onPromoted }: Props) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const gate = promoteGate(cluster, isAdmin);

  const promote = async () => {
    setBusy(true);
    try {
      const res = await fetch("/api/replication/promote", { method: "POST" });
      const body = (await res.json().catch(() => ({}))) as { ok?: boolean; code?: string; error?: string };
      if (!res.ok || !body.ok) throw new Error(promoteErrorMessage(body.code, body.error ?? `HTTP ${res.status}`));
      onNotice("Promoted to leader. The old leader was fenced and refuses writes.");
      onPromoted();
    } catch (e) {
      onNotice(`Promote failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  };

  const contact =
    cluster.leaderContactMS < 0 ? "never" : `${(cluster.leaderContactMS / 1000).toFixed(1)} s ago`;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-sm">Promotion</CardTitle>
        <Badge variant="mono">epoch {cluster.epoch}</Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Row label="Node" value={cluster.nodeID} />
          <Row label="Role" value={cluster.role} />
          {cluster.role === "follower" && (
            <>
              <Row label="Applied LSN" value={cluster.appliedLSN.toLocaleString()} />
              <Row label="Leader LSN (last heard)" value={cluster.leaderLSN.toLocaleString()} />
              <Row label="Lag (records)" value={cluster.lagLSN.toLocaleString()} />
              <Row label="Last heard from leader" value={contact} />
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button size="sm" variant="outline" disabled={!gate.enabled || busy} onClick={() => setConfirming(true)}>
            Promote to leader
          </Button>
          {!gate.enabled && (
            <p className="text-xs text-muted-foreground" data-testid="promote-reason">
              {gate.reason}
            </p>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Promotion is manual. It first fences the old leader (it stops accepting writes) and waits until this node has
          every record the leader acknowledged. If the leader cannot be reached, Studio refuses; forcing it is possible
          only through the API and can lose writes. Failover is not automatic and this is not high availability.
        </p>
      </CardContent>
      <AlertDialog open={confirming} onOpenChange={(o) => !busy && setConfirming(o)}>
        <AlertDialogContent>
          <AlertDialogTitle className="text-base font-semibold">Promote {cluster.nodeID} to leader?</AlertDialogTitle>
          <AlertDialogDescription className="mt-2 text-sm text-muted-foreground">
            The current leader will be fenced at epoch {cluster.epoch + 1} and refuse writes. Clients and other followers
            must be pointed at this node. The old leader cannot rejoin until its data directory is wiped and it is started
            as a follower.
          </AlertDialogDescription>
          <div className="mt-5 flex justify-end gap-2">
            <AlertDialogCancel asChild>
              <Button variant="outline" size="sm" disabled={busy}>
                Cancel
              </Button>
            </AlertDialogCancel>
            <AlertDialogAction asChild>
              <Button size="sm" disabled={busy} onClick={(e) => { e.preventDefault(); void promote(); }}>
                {busy ? "Promoting…" : "Promote"}
              </Button>
            </AlertDialogAction>
          </div>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
