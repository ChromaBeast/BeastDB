import { DbRecord } from "../types";

export function parseRecordValue(raw: string): Record<string, any> | null {
  const trimmed = raw.trim();
  if ((trimmed.startsWith("{") && trimmed.endsWith("}")) || (trimmed.startsWith("[") && trimmed.endsWith("]"))) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return null;
    }
  }
  return null;
}

export function enhanceRecord(rec: { key: number; value: string }): DbRecord {
  const parsed = parseRecordValue(rec.value);
  return {
    key: rec.key,
    value: rec.value,
    parsed: parsed || undefined,
  };
}

// 64-bit FNV-1a hash approximation for generating keys from identifiers/titles
export function fnv1a64(str: string): number {
  let hash = BigInt("0xcbf29ce484222325");
  const prime = BigInt("0x100000001b3");
  const mask = BigInt("0xffffffffffffffff");

  for (let i = 0; i < str.length; i++) {
    hash ^= BigInt(str.charCodeAt(i));
    hash = (hash * prime) & mask;
  }
  // Return positive safe integer for JS
  return Number(hash & BigInt("0x7fffffffffffffff"));
}

export function formatKey(key: number | string): string {
  const str = String(key);
  if (str.length > 12) {
    return str.slice(0, 4) + "..." + str.slice(-4);
  }
  return str;
}
