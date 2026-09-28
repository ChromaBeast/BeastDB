import { expect, test } from "bun:test";
import { parseUniversalRecord } from "./data-parser";
import { inferCollectionName, inferPartitionMeta } from "./schema-inference";
import { buildPartitionList, setInferredPartitionRegistry, setPartitionRegistry } from "./key-decoder";

test("infers Users collection from user records", () => {
  const rec = parseUniversalRecord({
    keyText: "72057594037927937",
    value: JSON.stringify({ username: "alice", email: "alice@example.com" }),
  });
  expect(inferCollectionName([rec])).toBe("Users");
});

test("infers System Users from internal user record without email", () => {
  const rec = parseUniversalRecord({
    keyText: "5836657920172081152",
    value: JSON.stringify({ username: "admin", role: "admin", created_at: "2026-01-01T00:00:00Z" }),
  });
  expect(inferCollectionName([rec])).toBe("System Users");
});

test("infers Products collection from SKU and price", () => {
  const rec = parseUniversalRecord({
    keyText: "144115188075855873",
    value: JSON.stringify({ sku: "PROD-01", price: 100, name: "Storage" }),
  });
  expect(inferCollectionName([rec])).toBe("Products");
});

test("infers Games collection from gameId attribute", () => {
  const rec = parseUniversalRecord({
    keyText: "216172782113783808",
    value: JSON.stringify({ gameId: "g-1", title: "Hollow Knight" }),
  });
  expect(inferCollectionName([rec])).toBe("Games");
});

test("infers Movies collection from movieId attribute", () => {
  const rec = parseUniversalRecord({
    keyText: "288230376151711744",
    value: JSON.stringify({ movieId: "m-1", title: "Inception" }),
  });
  expect(inferCollectionName([rec])).toBe("Movies");
});

test("infers Media collection from mediaId attribute", () => {
  const rec = parseUniversalRecord({
    keyText: "1152921504606846977",
    value: JSON.stringify({ mediaId: "MOV-001", title: "Interstellar" }),
  });
  expect(inferCollectionName([rec])).toBe("Media");
});

test("infers collection from explicit collection or type tag", () => {
  const rec = parseUniversalRecord({
    keyText: "360287970189639680",
    value: JSON.stringify({ type: "order", order_no: "12345" }),
  });
  expect(inferCollectionName([rec])).toBe("Orders");
});

test("fallback is neutral when no schema can be inferred", () => {
  const meta = inferPartitionMeta([], 9);
  expect(meta.label).toBe("Collection 09");
  expect(typeof meta.color).toBe("string");
});

test("relationship IDs do not mislabel records as users or senders", () => {
  const game = parseUniversalRecord({
    keyText: "216172782113783809",
    value: JSON.stringify({ userId: "u-1", gameId: "g-1", title: "Brawlhalla" }),
  });
  const request = parseUniversalRecord({
    keyText: "648518346341351425",
    value: JSON.stringify({ senderId: "u-1", receiverId: "u-2", status: "pending" }),
  });
  expect(inferCollectionName([game])).toBe("Games");
  expect(inferCollectionName([request])).toBe("Friend Requests");
});

test("explicit collection and nested subject outrank generic relationship fields", () => {
  const explicit = parseUniversalRecord({
    keyText: "648518346341351426",
    value: JSON.stringify({ collection: "friend_request", senderId: "u-1", receiverId: "u-2" }),
  });
  const nested = parseUniversalRecord({
    keyText: "216172782113783810",
    value: JSON.stringify({ user: { id: "u-1", name: "Alice" }, game: { id: "g-1", title: "Brawlhalla" } }),
  });
  expect(inferCollectionName([explicit])).toBe("Friend Requests");
  expect(inferCollectionName([nested])).toBe("Games");
});

test("infers Friendships from friendship records with userId and friendId", () => {
  const rec = parseUniversalRecord({
    keyText: "576460752303423488",
    value: JSON.stringify({ userId: "u-demo-1", friendId: "u-2", addedAt: "2026-09-28T00:00:00Z" }),
  });
  expect(inferCollectionName([rec])).toBe("Friendships");
});

test("infers Tokens from token payloads and strings", () => {
  const tokenRec = parseUniversalRecord({
    keyText: "360287970189639680",
    value: "u-demo-1|1738291029",
  });
  expect(inferCollectionName([tokenRec])).toBe("Tokens");

  const jsonToken = parseUniversalRecord({
    keyText: "360287970189639681",
    value: JSON.stringify({ tokenHash: "abc12345", expiresAt: 1738291029 }),
  });
  expect(inferCollectionName([jsonToken])).toBe("Tokens");
});

test("infers User Index from raw email pointer strings", () => {
  const emailIndex = parseUniversalRecord({
    keyText: "144115188075855872",
    value: "alice@example.com",
  });
  expect(inferCollectionName([emailIndex])).toBe("User Index");
});

test("a sampled partition is named even when absent from the loaded page", () => {
  setPartitionRegistry([]);
  setInferredPartitionRegistry({ 9: inferPartitionMeta([
    parseUniversalRecord({ keyText: "648518346341351425", value: JSON.stringify({ orderId: "o-1", total: 12 }) }),
  ], 9) });
  const list = buildPartitionList([], { "9": 4 });
  expect(list).toEqual([{ prefix: 9, count: 4, prefixLabel: "Orders" }]);
  setInferredPartitionRegistry({});
});

test("duplicate collection names remain distinguishable", () => {
  setPartitionRegistry([]);
  setInferredPartitionRegistry({
    1: { label: "Users", color: "blue" },
    2: { label: "Users", color: "red" },
  });
  const list = buildPartitionList([], { "1": 2, "2": 3 });
  expect(list.map((part) => part.prefixLabel)).toEqual(["Users · 01", "Users · 02"]);
  setInferredPartitionRegistry({});
});
