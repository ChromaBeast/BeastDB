"use client";
import { useCallback, useEffect, useState } from "react";
import { UniversalRecord } from "../types";
import { getPartitionMeta } from "../utils/key-decoder";
import { inferCollectionName } from "../utils/schema-inference";

export interface UserCollectionResult {
  prefix: number;
  label: string;
  color: string;
  records: UniversalRecord[];
  loading: boolean;
}

type ScanFn = (start: string, end: string) => Promise<UniversalRecord[]>;

// Scan all partitions that actually have data (from partitionCounts), except the
// user's own identity partition — so we discover collections dynamically from
// real data rather than any hardcoded schema.
export function useUserContext(
  userHash: number | undefined,
  identityPrefix: number,
  partitionCounts: Record<string, number> | undefined,
  onScanRange: ScanFn,
) {
  const [collections, setCollections] = useState<UserCollectionResult[]>([]);
  const [ready, setReady] = useState(false);

  const load = useCallback(async () => {
    if (userHash == null || !partitionCounts) return;
    const prefixes = Object.keys(partitionCounts)
      .map(Number)
      .filter((p) => p !== identityPrefix && partitionCounts[String(p)] > 0);
    if (prefixes.length === 0) { setReady(true); return; }

    setCollections(prefixes.map((prefix) => {
      const meta = getPartitionMeta(prefix);
      return { prefix, label: meta.label, color: meta.color, records: [], loading: true };
    }));
    setReady(false);

    // Fan out one bounded B+ Tree range scan per partition — all parallel
    const results = await Promise.all(
      prefixes.map(async (prefix): Promise<UserCollectionResult> => {
        const meta = getPartitionMeta(prefix);
        const base = (BigInt(prefix) << 56n) | (BigInt(userHash) << 28n);
        const start = base.toString();
        const end = (base | 0x0fffffffn).toString();
        try {
          const records = await onScanRange(start, end);
          const inferred = inferCollectionName(records);
          const label = (meta.label.startsWith("Partition 0x") && inferred) ? inferred : meta.label;
          return { prefix, label, color: meta.color, records, loading: false };
        } catch {
          return { prefix, label: meta.label, color: meta.color, records: [], loading: false };
        }
      }),
    );

    setCollections(results.filter((r) => r.records.length > 0).sort((a, b) => a.prefix - b.prefix));
    setReady(true);
  }, [userHash, identityPrefix, partitionCounts, onScanRange]);

  useEffect(() => { void load(); }, [load]);

  return { collections, ready };
}
