"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Logs page.
export function LogsView(_props: ViewProps) {
  return (
    <section aria-labelledby="logs-title" className="space-y-6">
      <PageHeader id="logs-title" title="Logs" subtitle="Coming together." />
    </section>
  );
}
