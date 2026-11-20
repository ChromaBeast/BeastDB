"use client";
import { useCallback, useEffect, useState } from "react";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";

/** Admin controls: require two-factor for administrators, and reset another user's second factor. */
export function TwoFactorAdminCard({ onNotice }: { onNotice: (message: string, error?: boolean) => void }) {
  const [required, setRequired] = useState<boolean | null>(null);
  const [username, setUsername] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await fetch("/api/settings/runtime", { cache: "no-store" });
      if (!res.ok) throw new Error(String(res.status));
      const rt = (await res.json()) as { values: Record<string, number> };
      setRequired(rt.values.require_2fa_admins === 1);
    } catch {
      setRequired(null);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  if (required === null) return null; // older server without the setting

  const toggle = async (next: boolean) => {
    setBusy(true);
    try {
      const res = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ require_2fa_admins: next ? 1 : 0 }),
      });
      if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
      setRequired(next);
      onNotice(next ? "Administrators must now use two-factor sign-in." : "Two-factor is optional for administrators again.");
    } catch (e) {
      onNotice(`Could not save: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusy(false);
    }
  };

  const reset = async () => {
    setBusy(true);
    try {
      const res = await fetch("/api/users/2fa-reset", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim() }),
      });
      if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
      onNotice(`Two-factor removed for ${username.trim()}. They can enrol again.`);
      setUsername("");
    } catch (e) {
      onNotice(`Could not reset: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Two-factor administration</CardTitle>
        <CardDescription>Applies to Studio sign-in only. Off by default so existing deployments keep working.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        <label className="flex items-start gap-3">
          <input type="checkbox" className="mt-1 h-4 w-4" checked={required} disabled={busy} onChange={(e) => void toggle(e.target.checked)} />
          <span>
            Require two-factor for administrators
            <span className="block text-xs text-muted-foreground">An administrator without it must set it up on the sign-in page before getting a session. Sessions already signed in stay valid until they expire.</span>
          </span>
        </label>
        <div className="space-y-1.5">
          <span className="text-muted-foreground">Reset another user&apos;s two-factor (lost device and recovery codes). Recorded in the audit log.</span>
          <div className="flex items-center gap-2">
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Username" aria-label="Username to reset" className="h-9 w-56" />
            <Button variant="outline" size="sm" disabled={busy || username.trim() === ""} onClick={() => void reset()}>Reset two-factor</Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
