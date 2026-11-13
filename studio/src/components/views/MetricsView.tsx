"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Metrics page.
export function MetricsView(_props: ViewProps) {
  return (
    <section aria-labelledby="metrics-title" className="space-y-6">
      <PageHeader id="metrics-title" title="Metrics" subtitle="Coming together." />
    </section>
  );
}
