import { describe, expect, test } from "bun:test";
import { formatAckAge, replicaState, summarizeReplicas } from "./replicas";
import type { ReplicaStatus } from "../types";

const rep = (over: Partial<ReplicaStatus> = {}): ReplicaStatus => ({
  id: "r1",
  connected: true,
  ackedLSN: 10,
  lagLSN: 0,
  lastAckAgoMS: 400,
  ...over,
});

describe("replicaState", () => {
  test("caught up, lagging, silent and disconnected", () => {
    expect(replicaState(rep())).toBe("streaming");
    expect(replicaState(rep({ lagLSN: 3 }))).toBe("lagging");
    expect(replicaState(rep({ lastAckAgoMS: 9000 }))).toBe("silent");
    expect(replicaState(rep({ lastAckAgoMS: -1 }))).toBe("silent");
    expect(replicaState(rep({ connected: false, lagLSN: 3 }))).toBe("disconnected");
  });
});

test("summarizeReplicas counts connected, lagging and disconnected", () => {
  expect(
    summarizeReplicas([rep(), rep({ lagLSN: 2 }), rep({ lastAckAgoMS: 20000 }), rep({ connected: false })]),
  ).toEqual({ connected: 3, lagging: 2, disconnected: 1 });
});

test("formatAckAge", () => {
  expect(formatAckAge(-1)).toBe("never");
  expect(formatAckAge(200)).toBe("just now");
  expect(formatAckAge(4200)).toBe("4 s ago");
  expect(formatAckAge(180000)).toBe("3 min ago");
});
