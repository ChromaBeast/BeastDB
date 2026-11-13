"use client";
import { PageHeader } from "../PageHeader";
import type { ViewProps } from "./types";

// TODO: replaced by the real Settings page.
export function SettingsView(_props: ViewProps) {
  return (
    <section aria-labelledby="settings-title" className="space-y-6">
      <PageHeader id="settings-title" title="Settings" subtitle="Coming together." />
    </section>
  );
}
