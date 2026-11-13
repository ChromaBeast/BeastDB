"use client";
import { useState } from "react";
import { Check, Copy, X } from "lucide-react";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { Input } from "../ui/input";
import { EXPIRY_OPTIONS, TOKEN_ROLES, type CreateTokenInput } from "../../hooks/useApiTokens";

const SELECT = "h-9 w-full rounded-md border border-border bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function CreateKeyForm({ onCreate, onCancel }: { onCreate: (i: CreateTokenInput) => Promise<void>; onCancel: () => void }) {
  const [name, setName] = useState("");
  const [role, setRole] = useState<string>("viewer");
  const [expiry, setExpiry] = useState(0);
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    await onCreate({ name, role, expiresInSeconds: expiry });
    setBusy(false);
  };

  return (
    <Card className="p-5">
      <form onSubmit={submit} className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">Create key</h2>
          <button type="button" onClick={onCancel} aria-label="Close form" className="text-muted-foreground hover:text-foreground">
            <X size={14} />
          </button>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <label className="space-y-1.5 text-xs text-muted-foreground sm:col-span-1">
            Name
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. analytics-service" maxLength={100} autoFocus required />
          </label>
          <label className="space-y-1.5 text-xs text-muted-foreground">
            Role
            <select value={role} onChange={(e) => setRole(e.target.value)} className={SELECT}>
              {TOKEN_ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
            </select>
          </label>
          <label className="space-y-1.5 text-xs text-muted-foreground">
            Expires
            <select value={expiry} onChange={(e) => setExpiry(Number(e.target.value))} className={SELECT}>
              {EXPIRY_OPTIONS.map((o) => <option key={o.seconds} value={o.seconds}>{o.label}</option>)}
            </select>
          </label>
        </div>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={onCancel}>Cancel</Button>
          <Button type="submit" size="sm" disabled={!name.trim() || busy}>{busy ? "Creating…" : "Create key"}</Button>
        </div>
      </form>
    </Card>
  );
}

/** One-time secret display. The parent must drop the secret when dismissed; it is never retrievable again. */
export function SecretReveal({ secret, onDismiss }: { secret: string; onDismiss: () => void }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be blocked; the secret stays selectable below.
    }
  };
  return (
    <Card className="border-primary/40 bg-primary/5 p-5" role="status">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-foreground">Copy your new key now</h2>
        <span className="text-xs text-muted-foreground">It is shown once and cannot be retrieved later.</span>
      </div>
      <div className="mt-3 flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
        <code className="min-w-0 flex-1 select-all break-all font-mono text-xs text-foreground">{secret}</code>
        <Button size="sm" variant="outline" onClick={copy} aria-label="Copy key">
          {copied ? <Check size={14} /> : <Copy size={14} />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" variant="ghost" onClick={onDismiss}>I have saved it</Button>
      </div>
    </Card>
  );
}
