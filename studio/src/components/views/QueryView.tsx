"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Query page.
export function QueryView(_props: ViewProps) {
  return (
    <section aria-labelledby="query-title" className="space-y-6">
      <PageHeader id="query-title" title="Query" subtitle="Coming together." />
    </section>
  );
}
