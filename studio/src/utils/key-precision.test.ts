import { expect, test } from "bun:test";
import { decodeKey } from "./key-decoder";
import { fnv1a64 } from "./format";
import { parseUniversalRecord } from "./data-parser";

test("preserves a uint64 key beyond JavaScript safe integers", () => {
  const key = "18446744073709551615";
  const record = parseUniversalRecord({ keyText: key, value: '{"name":"Precision"}' });
  expect(record.keyStr).toBe(key);
  expect(record.keyHex).toBe("0xffffffffffffffff");
  expect(record.prefix).toBe(255);
});

test("hashing an identifier returns a precise decimal string", () => {
  const key = fnv1a64("user@example.com");
  expect(typeof key).toBe("string");
  expect(decodeKey(key).raw).toBe(key);
  expect(BigInt(key) <= 18446744073709551615n).toBe(true);
});
