"use client";
import { useEffect, useState } from "react";
import { api } from "../lib/api-client";
import { parseUniversalRecord } from "../utils/data-parser";
import { setInferredPartitionRegistry } from "../utils/key-decoder";
import { inferPartitionMeta, PartitionMeta } from "../utils/schema-inference";

type Page = { records: Array<{ keyText: string; value: string }> };

// Stats enumerate every populated prefix. Sample each prefix independently so
// labels do not depend on the first page of the currently selected collection.
export function usePartitionMetadata(partitionCounts?: Record<string, number>): number {
  const [revision, setRevision] = useState(0);
  const signature = JSON.stringify(partitionCounts ?? {});

  useEffect(() => {
    const controller = new AbortController();
    const prefixes = Object.entries(partitionCounts ?? {})
      .filter(([, count]) => count > 0)
      .map(([prefix]) => Number(prefix))
      .filter((prefix) => Number.isInteger(prefix) && prefix >= 0 && prefix <= 255);

    async function load() {
      const inferred: Record<number, PartitionMeta> = {};
      for (let offset = 0; offset < prefixes.length; offset += 8) {
        const batch = prefixes.slice(offset, offset + 8);
        await Promise.all(batch.map(async (prefix) => {
          const start = BigInt(prefix) << 56n;
          const end = ((BigInt(prefix) + 1n) << 56n) - 1n;
          try {
            const page = await api<Page>(`/api/records?start=${start}&end=${end}&limit=24`, { signal: controller.signal });
            if (controller.signal.aborted) return;
            inferred[prefix] = inferPartitionMeta(page.records.map(parseUniversalRecord), prefix);
          } catch {
            // A failed sample leaves its honest generic label in place.
          }
        }));
        if (controller.signal.aborted) return;
        setInferredPartitionRegistry({ ...inferred });
        setRevision((value) => value + 1);
      }
    }

    setInferredPartitionRegistry({});
    setRevision((value) => value + 1);
    void load();
    return () => controller.abort();
  }, [signature]);

  return revision;
}
