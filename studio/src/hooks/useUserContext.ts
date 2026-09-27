"use client";
import { useCallback, useEffect, useState } from "react";
import { UniversalRecord } from "../types";
import { getRegisteredPartitions } from "../utils/key-decoder";

// For a given userHash, scan all non-identity partitions (skip 0x01 = the user's own record).
const SKIP_PREFIXES = new Set([0x01, 0x02, 0x05, 0x0a, 0x0b, 0x0c]);

export interface UserCollectionResult {
  prefix: number;
  label: string;
  color: string;
  records: UniversalRecord[];
  loading: boolean;
}

type ScanFn = (start: string, end: string) => Promise<UniversalRecord[]>;

export function useUserContext(userHash: number | undefined, onScanRange: ScanFn) {
  const [collections, setCollections] = useState<UserCollectionResult[]>([]);
  const [ready, setReady] = useState(false);

  const load = useCallback(async () => {
    if (userHash == null) return;
    const partitions = getRegisteredPartitions().filter((p) => !SKIP_PREFIXES.has(p.prefix));

    // Initialise with loading state
    setCollections(partitions.map((p) => ({ prefix: p.prefix, label: p.label, color: p.color, records: [], loading: true })));
    setReady(false);

    // Fan out all scans in parallel — each is a single B+ Tree bounded range scan
    const results = await Promise.all(
      partitions.map(async (p): Promise<UserCollectionResult> => {
        const base = (BigInt(p.prefix) << 56n) | (BigInt(userHash) << 28n);
        const start = base.toString();
        const end = (base | 0x0fffffffn).toString();
        try {
          const records = await onScanRange(start, end);
          return { prefix: p.prefix, label: p.label, color: p.color, records, loading: false };
        } catch {
          return { prefix: p.prefix, label: p.label, color: p.color, records: [], loading: false };
        }
      }),
    );

    setCollections(results.filter((r) => r.records.length > 0));
    setReady(true);
  }, [userHash, onScanRange]);

  useEffect(() => { void load(); }, [load]);

  return { collections, ready };
}
