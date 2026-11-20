"use client";
import { useCallback, useEffect, useState } from "react";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";
import { groupSetupKey, looksLikeCode, twoFactorError } from "../../utils/twofactor";

interface Status {
  enabled: boolean;
  recoveryRemaining: number;
  requiredByPolicy: boolean;
}

type Mode = "idle" | "setup" | "disable" | "recovery";

/** The signed-in user's own TOTP two-factor: enrol, show recovery codes, disable, regenerate. */
export function TwoFactorCard({ onNotice }: { onNotice: (message: string, error?: boolean) => void }) {
  const [status, setStatus] = useState<Status | null>(null);
  const [failed, setFailed] = useState(false);
  const [mode, setMode] = useState<Mode>("idle");
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await fetch("/api/account/2fa", { cache: "no-store" });
      if (!res.ok) throw new Error(String(res.status));
      setStatus((await res.json()) as Status);
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  if (failed) return null; // older server without two-factor
  if (!status) return null;

  const post = async (path: string, body?: unknown) => {
    const res = await fetch(`/api/account/2fa/${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) throw new Error(twoFactorError(res.status, await res.text()));
    return res;
  };

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      onNotice(e instanceof Error ? e.message : "Request failed", true);
    } finally {
      setBusy(false);
    }
  };

  const start = () => run(async () => {
    setSetup((await (await post("setup")).json()) as { secret: string; uri: string });
    setCode("");
    setMode("setup");
  });
  const enable = () => run(async () => {
    const out = (await (await post("enable", { code })).json()) as { recoveryCodes: string[] };
    setCodes(out.recoveryCodes);
    setSetup(null); setMode("idle"); setCode("");
    onNotice("Two-factor sign-in is on.");
    await load();
  });
  const disable = () => run(async () => {
    await post("disable", { code });
    setMode("idle"); setCode("");
    onNotice("Two-factor sign-in is off.");
    await load();
  });
  const regenerate = () => run(async () => {
    const out = (await (await post("recovery", { code })).json()) as { recoveryCodes: string[] };
    setCodes(out.recoveryCodes);
    setMode("idle"); setCode("");
    onNotice("New recovery codes generated. The old ones no longer work.");
    await load();
  });

  const codeInput = (label: string) => (
    <label className="block space-y-1.5 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <Input value={code} onChange={(e) => setCode(e.target.value)} autoComplete="one-time-code" spellCheck={false} placeholder="123456" className="h-9 w-56 font-mono" />
    </label>
  );
  const cancel = () => { setMode("idle"); setSetup(null); setCode(""); };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Two-factor sign-in</CardTitle>
        <CardDescription>
          Time-based one-time codes (TOTP, SHA-1, 6 digits, 30 seconds) from an authenticator app, asked for after your password.
          The secret is stored in the user record: encrypted if the server runs with encryption at rest, plaintext in the data file otherwise.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p>
          Status: <strong>{status.enabled ? "On" : "Off"}</strong>
          {status.enabled && <span className="text-muted-foreground"> · {status.recoveryRemaining} recovery code{status.recoveryRemaining === 1 ? "" : "s"} left</span>}
          {status.requiredByPolicy && <span className="text-muted-foreground"> · required for administrators</span>}
        </p>

        {codes && (
          <div className="space-y-2 rounded-lg border border-border bg-muted/40 p-3">
            <p className="font-medium">Save these recovery codes now</p>
            <p className="text-xs text-muted-foreground">Each works once if you lose your authenticator. They are not shown again.</p>
            <ul className="grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-xs">{codes.map((c) => <li key={c}>{c}</li>)}</ul>
            <Button variant="outline" size="sm" onClick={() => setCodes(null)}>I saved them</Button>
          </div>
        )}

        {mode === "idle" && !status.enabled && <Button size="sm" disabled={busy} onClick={() => void start()}>Set up two-factor</Button>}
        {mode === "idle" && status.enabled && (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => { setMode("recovery"); setCode(""); }}>New recovery codes</Button>
            {!status.requiredByPolicy && <Button variant="outline" size="sm" onClick={() => { setMode("disable"); setCode(""); }}>Turn off</Button>}
          </div>
        )}

        {mode === "setup" && setup && (
          <div className="space-y-3">
            <p className="text-muted-foreground">Add an account in your authenticator app with this setup key (or open the link on a device that has one), then enter the code it shows.</p>
            <div>
              <p className="text-xs text-muted-foreground">Setup key</p>
              <p className="select-all break-all font-mono text-sm">{groupSetupKey(setup.secret)}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Setup link</p>
              <p className="select-all break-all font-mono text-xs">{setup.uri}</p>
            </div>
            {codeInput("Code from the app")}
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={cancel}>Cancel</Button>
              <Button size="sm" disabled={busy || !looksLikeCode(code)} onClick={() => void enable()}>Turn on</Button>
            </div>
          </div>
        )}

        {(mode === "disable" || mode === "recovery") && (
          <div className="space-y-3">
            <p className="text-muted-foreground">Enter a current code or an unused recovery code to confirm.</p>
            {codeInput("Code")}
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={cancel}>Cancel</Button>
              <Button size="sm" disabled={busy || !looksLikeCode(code)} onClick={() => void (mode === "disable" ? disable() : regenerate())}>
                {mode === "disable" ? "Turn off" : "Generate new codes"}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
