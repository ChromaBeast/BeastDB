import { expect, test } from "bun:test";
import { parseUniversalRecord } from "./data-parser";
import { inferCollectionName, inferPartitionMeta } from "./schema-inference";

test("infers Users collection from user records", () => {
  const rec = parseUniversalRecord({
    keyText: "72057594037927937",
    value: JSON.stringify({ username: "alice", email: "alice@example.com" }),
  });
  expect(inferCollectionName([rec])).toBe("Users");
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

test("infers Media Catalog collection from mediaId attribute", () => {
  const rec = parseUniversalRecord({
    keyText: "1152921504606846977",
    value: JSON.stringify({ mediaId: "MOV-001", title: "Interstellar" }),
  });
  expect(inferCollectionName([rec])).toBe("Media Catalog");
});

test("infers collection from explicit collection or type tag", () => {
  const rec = parseUniversalRecord({
    keyText: "360287970189639680",
    value: JSON.stringify({ type: "order", order_no: "12345" }),
  });
  expect(inferCollectionName([rec])).toBe("Orders");
});

test("fallback to generic Partition 0x hex when no heuristic matches", () => {
  const meta = inferPartitionMeta([], 9);
  expect(meta.label).toBe("Partition 0x09");
  expect(typeof meta.color).toBe("string");
});
