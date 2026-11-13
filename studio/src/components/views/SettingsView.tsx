"use client";
import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { useTheme } from "../../hooks/useTheme";
import { RuntimeSettingsCard } from "../settings/RuntimeSettings";
import { SettingsCard, settingsCards, type Settings, type SettingsSection } from "../settings/SettingsCards";
import { cn } from "@/lib/utils";
import type { ViewProps } from "./types";

const SECTIONS: { id: SettingsSection; label: string }[] = [
  { id: "general", label: "General" },
  { id: "storage", label: "Storage" },
  { id: "networking", label: "Networking" },
  { id: "security", label: "Security" },
  { id: "limits", label: "Limits" },
];

export function SettingsView({ user, onNotice }: ViewProps) {
  const { dark, toggle } = useTheme();
  const [section, setSection] = useState<SettingsSection>("general");
  const isAdmin = user?.role === "admin";
  const [settings, setSettings] = useState<Settings | null>(null);
  const [state, setState] = useState<"loading" | "ok" | "forbidden" | "error">(isAdmin ? "loading" : "forbidden");

  useEffect(() => {
    if (!isAdmin) {
      setState("forbidden");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch("/api/settings", { cache: "no-store" });
        if (cancelled) return;
        if (res.status === 403 || res.status === 401) return setState("forbidden");
        if (!res.ok) return setState("error");
        setSettings((await res.json()) as Settings);
        setState("ok");
      } catch {
        if (!cancelled) setState("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isAdmin]);

  return (
    <section aria-labelledby="settings-title" className="space-y-6">
      <PageHeader id="settings-title" title="Settings" subtitle="Startup configuration is read-only. A few settings can be changed live and are saved to the data directory." />

      <div className="grid gap-4 md:grid-cols-[200px_minmax(0,1fr)]">
        <nav aria-label="Settings sections" className="flex gap-1 overflow-x-auto md:flex-col md:self-start md:rounded-xl md:border md:border-border md:bg-card md:p-2">
          {SECTIONS.map((x) => (
            <button
              key={x.id}
              type="button"
              aria-current={section === x.id ? "page" : undefined}
              onClick={() => setSection(x.id)}
              className={cn(
                "shrink-0 rounded-lg px-3 py-2 text-left text-sm transition-colors",
                section === x.id ? "bg-primary/10 font-medium text-primary dark:bg-[#1e2b0a]" : "text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              {x.label}
            </button>
          ))}
        </nav>

        <div className="min-w-0 space-y-4">
          {section === "general" && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Appearance</CardTitle>
                <CardDescription>Stored in this browser only.</CardDescription>
              </CardHeader>
              <CardContent className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">Theme: {dark ? "Dark" : "Light"}</span>
                <Button variant="outline" size="sm" onClick={toggle}>
                  {dark ? <Sun className="h-3.5 w-3.5" aria-hidden /> : <Moon className="h-3.5 w-3.5" aria-hidden />}
                  Switch to {dark ? "light" : "dark"}
                </Button>
              </CardContent>
            </Card>
          )}

          {state === "loading" && <p className="text-sm text-muted-foreground">Loading settings…</p>}
          {state === "error" && <p className="text-sm text-destructive">Could not load settings. Reload to retry.</p>}
          {state === "forbidden" && section !== "general" && (
            <Card className="p-6 text-sm text-muted-foreground">Node settings are visible to administrators only.</Card>
          )}
          {state === "forbidden" && section === "general" && (
            <Card className="p-6 text-sm text-muted-foreground">Node identity is visible to administrators only.</Card>
          )}
          {isAdmin && section === "storage" && (
            <RuntimeSettingsCard
              title="Checkpointing"
              description="Adjustable without a restart. Saved settings override the startup flag."
              onNotice={onNotice}
              fields={[{ name: "wal_checkpoint_mb", label: "WAL checkpoint threshold", unit: "MiB", hint: "The server checkpoints and rotates the WAL when it grows past this size." }]}
            />
          )}
          {isAdmin && section === "security" && (
            <RuntimeSettingsCard
              title="Sign-in rate limit"
              description="Failed Studio sign-ins allowed per client address."
              onNotice={onNotice}
              fields={[
                { name: "login_max_attempts", label: "Failed attempts allowed", unit: "attempts" },
                { name: "login_window_seconds", label: "Within a window of", unit: "seconds" },
              ]}
            />
          )}
          {state === "ok" && settings && (
            <>
              <p className="rounded-lg border border-border bg-muted/40 px-4 py-3 text-sm text-muted-foreground">{settings.note}</p>
              {settingsCards(settings)
                .filter((c) => c.section === section)
                .map(({ section: _s, ...c }) => (
                  <SettingsCard key={c.title} {...c} />
                ))}
            </>
          )}
        </div>
      </div>
    </section>
  );
}
