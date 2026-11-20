"use client";
import { useCallback, useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { api } from "../../lib/api-client";
import type { CollectionConfig, IndexEntry } from "../../types";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Input } from "../ui/input";

const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
const FIELD_RE = /^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+){0,7}$/;
const selectCls = "h-9 rounded-md border border-input bg-background px-2 text-[13px]";

interface Props {
  canWrite: boolean;
  onNotice: (message: string, error?: boolean) => void;
  /** Called after the list loads, with the number of secondary indexes. */
  onCount?: (n: number) => void;
}

/** Secondary indexes (ADR 0005): shows only what /api/indexes reports. */
export function FieldIndexes({ canWrite, onNotice, onCount }: Props) {
  const [indexes, setIndexes] = useState<IndexEntry[] | null>(null);
  const [collections, setCollections] = useState<CollectionConfig[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [collection, setCollection] = useState("");
  const [field, setField] = useState("");
  const [type, setType] = useState<"int" | "string">("int");
  const [busy, setBusy] = useState(false);
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [i, c] = await Promise.all([api<IndexEntry[]>("/api/indexes"), api<CollectionConfig[]>("/api/collections")]);
      setIndexes(i ?? []);
      setCollections(c ?? []);
      setError(null);
      onCount?.((i ?? []).length);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [onCount]);

  useEffect(() => { void load(); }, [load]);

  // An index that is still building reports progress; refresh until none is.
  const building = indexes?.some((i) => i.state !== "ready") ?? false;
  useEffect(() => {
    if (!building) return;
    const t = window.setInterval(() => void load(), 1500);
    return () => window.clearInterval(t);
  }, [building, load]);

  const valid = NAME_RE.test(name) && collection !== "" && FIELD_RE.test(field) && field.length <= 128;

  const create = async () => {
    if (!valid || busy) return;
    setBusy(true);
    try {
      await api("/api/indexes", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, collection, field, type }) });
      onNotice(`Index "${name}" created; it is building in the background.`);
      setName(""); setField(""); setOpen(false);
      await load();
    } catch (e) {
      onNotice((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  };

  const drop = async (n: string) => {
    try {
      await api(`/api/indexes?name=${encodeURIComponent(n)}`, { method: "DELETE" });
      onNotice(`Index "${n}" dropped.`);
      setConfirmDrop(null);
      await load();
    } catch (e) {
      onNotice((e as Error).message, true);
    }
  };

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
        <div>
          <CardTitle className="text-base">Field indexes</CardTitle>
          <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
            Single-field indexes over JSON records, one collection each. Entries are held in memory and rebuilt from the records after a restart;
            only records whose field exists with the declared type are indexed. Skipped counts the rest.
          </p>
        </div>
        {canWrite && !open && (
          <Button size="sm" variant="outline" onClick={() => setOpen(true)} disabled={collections.length === 0}
            title={collections.length === 0 ? "Create a collection first" : undefined}>
            <Plus className="mr-1.5 h-3.5 w-3.5" aria-hidden />New index
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {error && <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>}

        {open && (
          <form className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1fr_1.2fr_110px_auto]" onSubmit={(e) => { e.preventDefault(); void create(); }}>
            <div>
              <label htmlFor="idx-name" className="mb-1 block text-xs text-muted-foreground">Name</label>
              <Input id="idx-name" autoFocus value={name} onChange={(e) => setName(e.target.value.toLowerCase())} placeholder="e.g. users-by-age" />
            </div>
            <div>
              <label htmlFor="idx-col" className="mb-1 block text-xs text-muted-foreground">Collection</label>
              <select id="idx-col" className={`${selectCls} w-full`} value={collection} onChange={(e) => setCollection(e.target.value)}>
                <option value="">Choose…</option>
                {collections.map((c) => <option key={c.name} value={c.name}>{c.name}</option>)}
              </select>
            </div>
            <div>
              <label htmlFor="idx-field" className="mb-1 block text-xs text-muted-foreground">JSON field</label>
              <Input id="idx-field" value={field} onChange={(e) => setField(e.target.value)} placeholder="age or address.city" className="font-mono" />
            </div>
            <div>
              <label htmlFor="idx-type" className="mb-1 block text-xs text-muted-foreground">Type</label>
              <select id="idx-type" className={`${selectCls} w-full`} value={type} onChange={(e) => setType(e.target.value as "int" | "string")}>
                <option value="int">int</option>
                <option value="string">string</option>
              </select>
            </div>
            <div className="flex items-end gap-2">
              <Button type="submit" size="sm" disabled={!valid || busy}>{busy ? "Creating…" : "Create index"}</Button>
              <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            </div>
            <p className="text-xs text-muted-foreground sm:col-span-5">
              {name && !NAME_RE.test(name) ? "Name: a-z, 0-9, - and _ (max 64). " : ""}
              {field && !FIELD_RE.test(field) ? "Field: dotted path of letters, digits and _ (max 8 levels, no array indexes). " : ""}
              Ints must be written as whole JSON numbers; strings are indexed up to 256 bytes.
            </p>
          </form>
        )}

        {indexes === null && !error && <p className="text-sm text-muted-foreground">Loading…</p>}
        {indexes !== null && indexes.length === 0 && !error && (
          <p className="rounded-md border border-dashed border-border px-3 py-6 text-center text-sm text-muted-foreground">
            No field indexes yet.{canWrite ? " Create one to filter a collection by a JSON field." : " An admin can create one."}
          </p>
        )}
        {indexes !== null && indexes.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="py-2 pr-3 font-medium">Name</th>
                  <th className="py-2 pr-3 font-medium">Collection</th>
                  <th className="py-2 pr-3 font-medium">Field</th>
                  <th className="py-2 pr-3 font-medium">Type</th>
                  <th className="py-2 pr-3 font-medium">State</th>
                  <th className="py-2 pr-3 text-right font-medium">Entries</th>
                  <th className="py-2 pr-3 text-right font-medium">Skipped</th>
                  {canWrite && <th className="py-2 font-medium"><span className="sr-only">Actions</span></th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {indexes.map((i) => (
                  <tr key={i.name}>
                    <td className="py-2 pr-3 font-mono text-[13px]">{i.name}</td>
                    <td className="py-2 pr-3">{i.collection || <span className="text-muted-foreground">unbound 0x{i.prefix.toString(16)}</span>}</td>
                    <td className="py-2 pr-3 font-mono text-[13px]">{i.field}</td>
                    <td className="py-2 pr-3">{i.type}</td>
                    <td className="py-2 pr-3">
                      <Badge variant={i.state === "ready" ? "default" : "secondary"}>
                        {i.state === "ready" ? "ready" : `building · ${i.examined.toLocaleString()} read`}
                      </Badge>
                    </td>
                    <td className="py-2 pr-3 text-right font-mono tabular-nums">{i.entries.toLocaleString()}</td>
                    <td className="py-2 pr-3 text-right font-mono tabular-nums">{i.skipped.toLocaleString()}</td>
                    {canWrite && (
                      <td className="py-2 text-right">
                        {confirmDrop === i.name ? (
                          <span className="inline-flex items-center gap-1">
                            <Button size="sm" variant="destructive" onClick={() => void drop(i.name)}>Drop {i.name}</Button>
                            <Button size="sm" variant="ghost" onClick={() => setConfirmDrop(null)}>Cancel</Button>
                          </span>
                        ) : (
                          <Button size="icon" variant="ghost" className="h-7 w-7" aria-label={`Drop index ${i.name}`} onClick={() => setConfirmDrop(i.name)}>
                            <Trash2 size={14} />
                          </Button>
                        )}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
