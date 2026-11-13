"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Backups page.
export function BackupsView(_props: ViewProps) {
  return (
    <section aria-labelledby="backups-title" className="space-y-6">
      <PageHeader id="backups-title" title="Backups" subtitle="Coming together." />
    </section>
  );
}
