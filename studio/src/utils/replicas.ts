import type { ReplicaStatus } from "../types";

export type ReplicaState = "streaming" | "lagging" | "silent" | "disconnected";

/** A connected replica that has not acknowledged anything for this long is not healthy (heartbeat acks come about once a second). */
export const SILENT_AFTER_MS = 5000;

/**
 * Classify a replica from what the leader really knows: whether its WAL stream
 * is open, how far its last acknowledged LSN is behind the leader, and how long
 * ago it last acknowledged. Lag is only as fresh as that last acknowledgement.
 */
export function replicaState(r: ReplicaStatus): ReplicaState {
  if (!r.connected) return "disconnected";
  if (r.lastAckAgoMS < 0 || r.lastAckAgoMS > SILENT_AFTER_MS) return "silent";
  return r.lagLSN > 0 ? "lagging" : "streaming";
}

export function replicaStateLabel(state: ReplicaState): string {
  switch (state) {
    case "streaming":
      return "caught up";
    case "lagging":
      return "lagging";
    case "silent":
      return "no recent ack";
    default:
      return "disconnected";
  }
}

export function formatAckAge(ms: number): string {
  if (ms < 0) return "never";
  if (ms < 1000) return "just now";
  if (ms < 60_000) return `${Math.round(ms / 1000)} s ago`;
  return `${Math.round(ms / 60_000)} min ago`;
}

/** Counts for the summary line above the table. */
export function summarizeReplicas(rs: ReplicaStatus[]): { connected: number; lagging: number; disconnected: number } {
  let connected = 0;
  let lagging = 0;
  let disconnected = 0;
  for (const r of rs) {
    const s = replicaState(r);
    if (s === "disconnected") disconnected++;
    else connected++;
    if (s === "lagging" || s === "silent") lagging++;
  }
  return { connected, lagging, disconnected };
}
