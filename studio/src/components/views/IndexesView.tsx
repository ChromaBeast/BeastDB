"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Indexes page.
export function IndexesView(_props: ViewProps) {
  return (
    <section aria-labelledby="indexes-title" className="space-y-6">
      <PageHeader id="indexes-title" title="Indexes" subtitle="Coming together." />
    </section>
  );
}
