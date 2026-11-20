import { describe, expect, test } from "bun:test";
import { promoteErrorMessage, promoteGate, rejoinErrorMessage, validLeaderAddr } from "./promote";
import type { ClusterStatus } from "../types";

const follower = (over: Partial<ClusterStatus> = {}): ClusterStatus => ({
  nodeID: "r1",
  role: "follower",
  epoch: 2,
  appliedLSN: 100,
  leaderLSN: 100,
  leaderContactMS: 120,
  lagLSN: 0,
  promotable: true,
  ...over,
});

describe("promoteGate", () => {
  test("enabled only for an admin on a caught-up follower", () => {
    expect(promoteGate(follower(), true)).toEqual({ enabled: true, reason: "" });
  });

  test("disabled with the server's reason when behind", () => {
    const g = promoteGate(follower({ promotable: false, lagLSN: 4, promoteBlockedReason: "Behind the leader by 4 records" }), true);
    expect(g.enabled).toBe(false);
    expect(g.reason).toContain("Behind the leader by 4 records");
  });

  test("disabled with a reason when the server gives none", () => {
    const g = promoteGate(follower({ promotable: false }), true);
    expect(g.enabled).toBe(false);
    expect(g.reason).not.toBe("");
  });

  test("viewers, leaders, fenced nodes and unknown state are all disabled with a reason", () => {
    for (const g of [
      promoteGate(follower(), false),
      promoteGate(follower({ role: "leader", promotable: false }), true),
      promoteGate(follower({ role: "fenced", promotable: false }), true),
      promoteGate(undefined, true),
    ]) {
      expect(g.enabled).toBe(false);
      expect(g.reason.length).toBeGreaterThan(0);
    }
  });

  test("a fenced node's reason names its epoch", () => {
    expect(promoteGate(follower({ role: "fenced", epoch: 7, promotable: false }), true).reason).toContain("7");
  });
});

describe("promoteErrorMessage", () => {
  test("explains an unreachable leader without suggesting a one-click override", () => {
    const m = promoteErrorMessage("leader_unreachable", "x");
    expect(m).toContain("lose");
    expect(m).toContain("acceptDataLoss");
  });
  test("falls back to the server text", () => {
    expect(promoteErrorMessage("failed", "disk full")).toBe("disk full");
  });
});

describe("rejoin helpers", () => {
  test("validLeaderAddr accepts host:port only", () => {
    expect(validLeaderAddr("10.0.0.2:50051")).toBe(true);
    expect(validLeaderAddr(" db1.internal:50051 ")).toBe(true);
    expect(validLeaderAddr("[::1]:50051")).toBe(true);
    for (const bad of ["", "db1", "http://db1:50051", "db1:50051/x", "db1:port", "a b:1"]) {
      expect(validLeaderAddr(bad)).toBe(false);
    }
  });

  test("a refusal says nothing was changed", () => {
    expect(rejoinErrorMessage("rejoin_refused", "holds 3 records the leader never had")).toContain("nothing was changed");
    expect(rejoinErrorMessage("not_fenced", "x")).toContain("not fenced");
    expect(rejoinErrorMessage(undefined, "boom")).toBe("boom");
  });
});
