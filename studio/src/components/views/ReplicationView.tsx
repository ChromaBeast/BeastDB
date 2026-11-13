"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Replication page.
export function ReplicationView(_props: ViewProps) {
  return (
    <section aria-labelledby="replication-title" className="space-y-6">
      <PageHeader id="replication-title" title="Replication" subtitle="Coming together." />
    </section>
  );
}
