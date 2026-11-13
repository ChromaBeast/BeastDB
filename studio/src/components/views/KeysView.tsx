"use client";
import { useState } from "react";
import { Key, Lock, Plus, Trash2 } from "lucide-react";
import { PageHeader } from "../PageHeader";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogTitle } from "../ui/alert-dialog";
import { CreateKeyForm, SecretReveal } from "../keys/CreateKeyForm";
import { RolesReference } from "../keys/RolesReference";
import { TokenRoleBadge } from "../keys/TokenRoleBadge";
import { isExpired, useApiTokens, type ApiToken } from "../../hooks/useApiTokens";
import { formatDateRelative } from "../../utils/format";
import type { ViewProps } from "./types";

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" }) : "—");

function ExpiryCell({ token }: { token: ApiToken }) {
  if (!token.expires_at) return <span className="text-muted-foreground">Never</span>;
  if (isExpired(token)) return <span className="text-destructive">Expired {fmtDate(token.expires_at)}</span>;
  return <span title={new Date(token.expires_at).toLocaleString()}>{fmtDate(token.expires_at)}</span>;
}

function KeysTable({ tokens, onRevoke }: { tokens: ApiToken[]; onRevoke: (t: ApiToken) => void }) {
  const sorted = [...tokens].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  return (
    <Card className="overflow-x-auto">
      <table className="w-full min-w-[640px] text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs text-muted-foreground">
            <th scope="col" className="px-5 py-3 font-medium">Name</th>
            <th scope="col" className="px-3 py-3 font-medium">Role</th>
            <th scope="col" className="px-3 py-3 font-medium">Key</th>
            <th scope="col" className="px-3 py-3 font-medium">Created</th>
            <th scope="col" className="px-3 py-3 font-medium">Expires</th>
            <th scope="col" className="px-5 py-3"><span className="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((t) => (
            <tr key={t.id} className="border-b border-border last:border-0">
              <td className="px-5 py-3">
                <span className="font-medium text-foreground">{t.name}</span>
                {t.created_by && <span className="block text-[11px] text-muted-foreground">by {t.created_by}</span>}
              </td>
              <td className="px-3 py-3"><TokenRoleBadge role={t.role} /></td>
              <td className="px-3 py-3 font-mono text-xs text-muted-foreground">{t.masked_token}</td>
              <td className="px-3 py-3 text-muted-foreground" title={new Date(t.created_at).toLocaleString()}>{formatDateRelative(t.created_at)}</td>
              <td className="px-3 py-3 font-mono text-xs"><ExpiryCell token={t} /></td>
              <td className="px-5 py-3 text-right">
                <Button size="sm" variant="ghost" onClick={() => onRevoke(t)} aria-label={`Revoke ${t.name}`} className="text-muted-foreground hover:text-destructive">
                  <Trash2 size={14} /> Revoke
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  );
}

export function KeysView({ user, onNotice }: ViewProps) {
  const admin = user?.role === "admin";
  const { tokens, loading, forbidden, error, create, revoke } = useApiTokens(admin, onNotice);
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [pending, setPending] = useState<ApiToken | null>(null);

  const locked = !admin || forbidden;

  const header = (
    <PageHeader
      id="keys-title"
      title="API Keys & Roles"
      subtitle="Bearer tokens for gRPC clients and scrapers. Stored in partition 0x05; only a hash is kept."
      actions={
        !locked && !creating ? (
          <Button size="sm" onClick={() => { setCreating(true); setSecret(null); }}>
            <Plus size={14} /> Create key
          </Button>
        ) : undefined
      }
    />
  );

  return (
    <section aria-labelledby="keys-title" className="space-y-6">
      {header}
      {locked ? (
        <Card className="flex flex-col items-center gap-2 px-6 py-14 text-center" role="status">
          <Lock size={22} className="text-muted-foreground" />
          <p className="text-sm font-semibold">Admin only</p>
          <p className="max-w-md text-xs text-muted-foreground">Viewer accounts cannot list, create or revoke API keys. Ask an administrator, or sign in as one.</p>
        </Card>
      ) : (
        <>
          {secret && <SecretReveal secret={secret} onDismiss={() => setSecret(null)} />}
          {creating && (
            <CreateKeyForm
              onCancel={() => setCreating(false)}
              onCreate={async (input) => {
                const s = await create(input);
                if (s) {
                  setSecret(s);
                  setCreating(false);
                }
              }}
            />
          )}
          {error && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">Could not load keys: {error}</p>}
          {tokens.length > 0 ? (
            <KeysTable tokens={tokens} onRevoke={setPending} />
          ) : (
            <Card className="flex flex-col items-center gap-2 border-dashed px-6 py-12 text-center">
              <Key size={22} className="text-muted-foreground" />
              <p className="text-sm font-semibold">{loading ? "Loading keys…" : "No API keys yet"}</p>
              {!loading && <p className="max-w-sm text-xs text-muted-foreground">Create a key to let services or scripts connect over gRPC with a limited role.</p>}
            </Card>
          )}
        </>
      )}
      <RolesReference />

      <AlertDialog open={!!pending} onOpenChange={(o) => { if (!o) setPending(null); }}>
        <AlertDialogContent>
          <AlertDialogTitle className="text-base font-semibold">Revoke this key?</AlertDialogTitle>
          <AlertDialogDescription className="mt-2 text-sm text-muted-foreground">
            Clients using <span className="font-mono text-foreground">{pending?.name}</span> will be rejected immediately. This cannot be undone.
          </AlertDialogDescription>
          <div className="mt-5 flex justify-end gap-2">
            <AlertDialogCancel asChild><Button variant="outline" size="sm">Cancel</Button></AlertDialogCancel>
            <AlertDialogAction asChild>
              <Button variant="destructive" size="sm" onClick={() => pending && void revoke(pending.id)}>Revoke key</Button>
            </AlertDialogAction>
          </div>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
