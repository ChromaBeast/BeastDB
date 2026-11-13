"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Keys page.
export function KeysView(_props: ViewProps) {
  return (
    <section aria-labelledby="keys-title" className="space-y-6">
      <PageHeader id="keys-title" title="Keys" subtitle="Coming together." />
    </section>
  );
}
