"use client";
import { useCallback, useEffect, useState } from "react";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";

interface Runtime {
  values: Record<string, number>;
  ranges: Record<string, { min: number; max: number }>;
}

export interface RuntimeField {
  /** JSON name used by /api/settings/runtime. */
  name: string;
  label: string;
  unit: string;
  hint?: string;
}

/** Editable card for the settings the server lets admins change without a restart. */
export function RuntimeSettingsCard({
  title, description, fields, onNotice,
}: {
  title: string;
  description: string;
  fields: RuntimeField[];
  onNotice: (message: string, error?: boolean) => void;
}) {
  const [rt, setRt] = useState<Runtime | null>(null);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  const apply = (r: Runtime) => {
    setRt(r);
    setDraft(Object.fromEntries(fields.map((f) => [f.name, String(r.values[f.name])])));
  };

  const load = useCallback(async () => {
    try {
      const res = await fetch("/api/settings/runtime", { cache: "no-store" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      apply((await res.json()) as Runtime);
      setFailed(false);
    } catch {
      setFailed(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => { void load(); }, [load]);

  if (failed) return null; // older server without the endpoint: keep the page read-only
  if (!rt) return null;

  const parsed = fields.map((f) => ({ f, n: Number(draft[f.name]), range: rt.ranges[f.name] }));
  const invalid = parsed.some(({ n, range }) => !Number.isInteger(n) || n < range.min || n > range.max);
  const dirty = parsed.some(({ f, n }) => n !== rt.values[f.name]);

  const save = async () => {
    setBusy(true);
    try {
      const patch = Object.fromEntries(parsed.filter(({ f, n }) => n !== rt.values[f.name]).map(({ f, n }) => [f.name, n]));
      const res = await fetch("/api/settings/runtime", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
      });
      if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
      apply((await res.json()) as Runtime);
      onNotice("Settings saved. They apply now and survive a restart.");
    } catch (e) {
      onNotice(`Could not save settings: ${e instanceof Error ? e.message : "request failed"}`, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {parsed.map(({ f, n, range }) => {
          const bad = !Number.isInteger(n) || n < range.min || n > range.max;
          return (
            <label key={f.name} className="block space-y-1.5 text-sm">
              <span className="text-muted-foreground">{f.label}</span>
              <span className="flex items-center gap-2">
                <Input
                  type="number"
                  inputMode="numeric"
                  value={draft[f.name] ?? ""}
                  min={range.min}
                  max={range.max}
                  onChange={(e) => setDraft((d) => ({ ...d, [f.name]: e.target.value }))}
                  aria-invalid={bad}
                  className="h-9 w-32 font-mono"
                />
                <span className="text-xs text-muted-foreground">{f.unit} · {range.min.toLocaleString()}–{range.max.toLocaleString()}</span>
              </span>
              {bad && <span className="block text-xs text-destructive">Enter a whole number from {range.min} to {range.max}.</span>}
              {f.hint && !bad && <span className="block text-xs text-muted-foreground">{f.hint}</span>}
            </label>
          );
        })}
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" disabled={!dirty || busy} onClick={() => apply(rt)}>Reset</Button>
          <Button size="sm" disabled={!dirty || invalid || busy} onClick={() => void save()}>{busy ? "Saving…" : "Save changes"}</Button>
        </div>
      </CardContent>
    </Card>
  );
}
