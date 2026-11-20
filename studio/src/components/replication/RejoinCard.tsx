"use client";
import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { rejoinErrorMessage, validLeaderAddr } from "../../utils/promote";
import type { ClusterStatus } from "../../types";

interface Props {
  cluster: ClusterStatus;
  onNotice: (message: string, error?: boolean) => void;
  onRejoined: () => void;
}

/**
 * Shown on a fenced former leader. Rejoining turns it into a follower of the
 * leader whose address is entered, but only when the node's log is provably a
 * prefix of that leader's; otherwise the server refuses and says what the node
 * holds that the leader lacks.
 */
export function RejoinCard({ cluster, onNotice, onRejoined }: Props) {
  const [addr, setAddr] = useState("");
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  const ok = validLeaderAddr(addr);

  const rejoin = async () => {
    setBusy(true);
    setRefusal(null);
    try {
      const res = await fetch("/api/replication/rejoin", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ leaderAddr: addr.trim() }),
      });
      const body = (await res.json().catch(() => ({}))) as { ok?: boolean; code?: string; error?: string };
      if (!res.ok || !body.ok) {
        const msg = rejoinErrorMessage(body.code, body.error ?? `HTTP ${res.status}`);
        if (body.code === "rejoin_refused") setRefusal(msg);
        throw new Error(msg);
      }
      onNotice("Rejoined as a follower. It is catching up from the new leader.");
      onRejoined();
    } catch (e) {
      onNotice(`Rejoin failed: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-sm">Rejoin as a follower</CardTitle>
        <Badge variant="mono">fenced at epoch {cluster.epoch}</Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          This node was a leader and was fenced by a promotion. It stays read-only. Enter the gRPC address of the
          current leader: the node compares its log with the leader&apos;s and rejoins only if every record it holds is
          also on that leader.
        </p>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            Current leader (host:port)
            <Input
              value={addr}
              onChange={(e) => setAddr(e.target.value)}
              placeholder="10.0.0.2:50051"
              className="w-64 font-mono"
              spellCheck={false}
            />
          </label>
          <Button size="sm" variant="outline" disabled={!ok || busy} onClick={() => void rejoin()}>
            {busy ? "Checking…" : "Rejoin as follower"}
          </Button>
        </div>
        {refusal && (
          <p className="rounded-md border border-destructive/40 p-3 text-xs text-destructive" role="alert" data-testid="rejoin-refusal">
            {refusal}
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          Refused means the node accepted writes the new leader never received. Nothing is changed or deleted; export
          those records first if you need them, then wipe the data directory to reseed. The same operation is
          available as <span className="font-mono">beastctl rejoin</span>, and every attempt is written to the audit log.
        </p>
      </CardContent>
    </Card>
  );
}
