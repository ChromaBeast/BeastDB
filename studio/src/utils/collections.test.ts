import { beforeEach, describe, expect, test } from "bun:test";
import { buildPartitionList, getCollectionName, getPartitionMeta, setCollectionRegistry, setPartitionRegistry } from "./key-decoder";

describe("collection registry", () => {
  beforeEach(() => {
    setCollectionRegistry([]);
    setPartitionRegistry([]);
  });

  test("catalog name overrides partitions.json label and keeps its color", () => {
    setPartitionRegistry([{ prefix: 16, label: "Entity Catalog", color: "orange" }]);
    setCollectionRegistry([{ name: "orders", prefix: 16, createdAt: 1 }]);
    expect(getPartitionMeta(16)).toMatchObject({ label: "orders", color: "orange" });
    expect(getCollectionName(16)).toBe("orders");
    expect(getCollectionName(17)).toBeUndefined();
  });

  test("named but empty collections still appear in the rail list with count 0", () => {
    setCollectionRegistry([{ name: "users", prefix: 16, createdAt: 1 }]);
    const list = buildPartitionList([], { "0": 3 });
    expect(list).toEqual([
      { prefix: 0, count: 3, prefixLabel: "Collection 00" },
      { prefix: 16, count: 0, prefixLabel: "users" },
    ]);
  });

  test("unnamed prefixes keep their generic label when the catalog is empty", () => {
    expect(getPartitionMeta(32).label).toBe("Collection 20");
  });
});
