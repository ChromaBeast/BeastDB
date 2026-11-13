"use client";
import { useEffect, useState } from "react";
import { Check, Copy, Moon, Sun } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { useTheme } from "../../hooks/useTheme";
import { formatBytes } from "../../utils/format";
import type { ViewProps } from "./types";

interface Settings {
  node: { role: string; version: string; replica_id: string; leader_addr: string; dev_mode: boolean };
  storage: { data_dir: string; pool_size_frames: number; page_size_bytes: number; wal_checkpoint_mb: number };
  network: { grpc_addr: string; web_addr: string };
  security: {
    tls_enabled: boolean;
    mutual_tls: boolean;
    api_token_configured: boolean;
    metrics_token_configured: boolean;
    insecure_auth_override: boolean;
    partition_config_path: string;
  };
  limits: {
    max_value_bytes: number;
    max_inline_record: number;
    max_batch_bytes: number;
    max_grpc_message: number;
    max_web_put_body: number;
    max_records_page_size: number;
  };
  note: string;
}

type Row = { label: string; value: string; copy?: boolean; flag?: boolean };

function CopyButton({ value, label }: { value: string; label: string }) {
  const [done, setDone] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setDone(true);
      setTimeout(() => setDone(false), 1500);
    } catch {
      /* clipboard unavailable (insecure context): nothing to do */
    }
  };
  return (
    <Button variant="ghost" size="icon" className="h-7 w-7" onClick={copy} aria-label={`Copy ${label}`}>
      {done ? <Check className="h-3.5 w-3.5" aria-hidden /> : <Copy className="h-3.5 w-3.5" aria-hidden />}
    </Button>
  );
}

function SettingsCard({ title, description, rows }: { title: string; description: string; rows: Row[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="divide-y divide-border">
          {rows.map((r) => (
            <div key={r.label} className="flex items-center justify-between gap-4 py-2">
              <dt className="text-sm text-muted-foreground">{r.label}</dt>
              <dd className="flex min-w-0 items-center gap-1">
                {r.flag ? (
                  <Badge variant={r.value === "Yes" ? "outline" : "mono"}>{r.value}</Badge>
                ) : (
                  <span className="truncate font-mono text-sm" title={r.value}>{r.value || "—"}</span>
                )}
                {r.copy && r.value && <CopyButton value={r.value} label={r.label} />}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}

const yn = (b: boolean) => (b ? "Yes" : "No");

function cards(s: Settings): { title: string; description: string; rows: Row[] }[] {
  return [
    {
      title: "Node",
      description: "Identity and role of this server.",
      rows: [
        { label: "Role", value: s.node.role },
        { label: "Version", value: s.node.version, copy: true },
        { label: "Replica ID", value: s.node.replica_id, copy: true },
        ...(s.node.leader_addr ? [{ label: "Leader address", value: s.node.leader_addr, copy: true }] : []),
        { label: "Dev emulator", value: yn(s.node.dev_mode), flag: true },
      ],
    },
    {
      title: "Storage",
      description: "Where data lives and how the engine buffers it.",
      rows: [
        { label: "Data directory", value: s.storage.data_dir, copy: true },
        { label: "Buffer pool", value: `${s.storage.pool_size_frames} frames (${formatBytes(s.storage.pool_size_frames * s.storage.page_size_bytes)})` },
        { label: "Page size", value: formatBytes(s.storage.page_size_bytes) },
        { label: "WAL checkpoint threshold", value: `${s.storage.wal_checkpoint_mb} MiB` },
      ],
    },
    {
      title: "Network",
      description: "Listening addresses.",
      rows: [
        { label: "gRPC", value: s.network.grpc_addr, copy: true },
        { label: "Web console", value: s.network.web_addr || "disabled", copy: !!s.network.web_addr },
      ],
    },
    {
      title: "Security",
      description: "Whether protections are configured. Secret values are never shown.",
      rows: [
        { label: "TLS on gRPC", value: yn(s.security.tls_enabled), flag: true },
        { label: "Client certificates (mTLS CA)", value: yn(s.security.mutual_tls), flag: true },
        { label: "gRPC API token set", value: yn(s.security.api_token_configured), flag: true },
        { label: "Metrics token set", value: yn(s.security.metrics_token_configured), flag: true },
        { label: "Insecure-auth override", value: yn(s.security.insecure_auth_override), flag: true },
        { label: "Partition config", value: s.security.partition_config_path || "none", copy: !!s.security.partition_config_path },
      ],
    },
    {
      title: "Limits",
      description: "Fixed by the engine; not configurable at runtime.",
      rows: [
        { label: "Max value size", value: formatBytes(s.limits.max_value_bytes) },
        { label: "Max inline record", value: formatBytes(s.limits.max_inline_record) },
        { label: "Max batch size", value: formatBytes(s.limits.max_batch_bytes) },
        { label: "Max gRPC message", value: formatBytes(s.limits.max_grpc_message) },
        { label: "Max web upload body", value: formatBytes(s.limits.max_web_put_body) },
        { label: "Records page size (max)", value: String(s.limits.max_records_page_size) },
      ],
    },
  ];
}

export function SettingsView({ user }: ViewProps) {
  const { dark, toggle } = useTheme();
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
      <PageHeader id="settings-title" title="Settings" subtitle="How this node is configured. Read-only." />

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

      {state === "loading" && <p className="text-sm text-muted-foreground">Loading settings…</p>}
      {state === "error" && <p className="text-sm text-destructive">Could not load settings. Reload to retry.</p>}
      {state === "forbidden" && (
        <Card className="p-6 text-sm text-muted-foreground">Node settings are visible to administrators only.</Card>
      )}
      {state === "ok" && settings && (
        <>
          <p className="rounded-lg border border-border bg-muted/40 px-4 py-3 text-sm text-muted-foreground">
            {settings.note}
          </p>
          <div className="grid gap-4 lg:grid-cols-2">
            {cards(settings).map((c) => (
              <SettingsCard key={c.title} {...c} />
            ))}
          </div>
        </>
      )}
    </section>
  );
}
