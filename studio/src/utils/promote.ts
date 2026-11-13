import type { ClusterStatus } from "../types";

export interface PromoteGate {
  enabled: boolean;
  /** Shown next to the disabled button; empty when enabled. */
  reason: string;
}

/**
 * Whether the Promote button may be used. Mirrors the server's `promotable`
 * flag, which is true only for a follower in recent contact with its leader
 * that has applied everything the leader has. The server fences the old
 * leader regardless, so this is a conservative gate, not the safety mechanism.
 */
export function promoteGate(cluster: ClusterStatus | undefined, isAdmin: boolean): PromoteGate {
  if (!isAdmin) return { enabled: false, reason: "Only admin users can promote a node." };
  if (!cluster) return { enabled: false, reason: "This node does not report cluster state." };
  if (cluster.role === "leader") return { enabled: false, reason: "This node is already the leader." };
  if (cluster.role === "fenced") {
    return { enabled: false, reason: `Fenced at epoch ${cluster.epoch}: rebuild this node as a follower before it can lead.` };
  }
  if (!cluster.promotable) {
    return { enabled: false, reason: cluster.promoteBlockedReason || "Not caught up with the leader." };
  }
  return { enabled: true, reason: "" };
}

/** Message for a failed promote call, using the server's error code. */
export function promoteErrorMessage(code: string | undefined, error: string): string {
  switch (code) {
    case "leader_unreachable":
      return "The leader did not answer, so it could not be fenced. Promoting now could lose its latest writes. If the leader is truly down, use the API with acceptDataLoss.";
    case "catch_up_timeout":
      return "The old leader is fenced but this node had not caught up in time. Nothing was promoted; try again.";
    case "fence_refused":
      return `The old leader refused to be fenced: ${error}`;
    default:
      return error;
  }
}
