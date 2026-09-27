"use client";
import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { Wand2 } from "lucide-react";
import { fnv1a64 } from "../utils/format";
import { getRegisteredPartitions } from "../utils/key-decoder";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "./ui/dialog";
import { buildFields, Field, RecordFields } from "./RecordFields";

interface Props {
  open: boolean;
  onClose: () => void;
  onSave: (key: string, value: string) => Promise<void>;
  onLookup: (key: string) => Promise<unknown>;
  onNotice: (message: string, error?: boolean) => void;
}

/** Compute a 28-bit FNV-1a hash as a BigInt. */
function fnv28(str: string): bigint {
  let h = BigInt("0xcbf29ce484222325");
  const prime = BigInt("0x100000001b3");
  const mask = BigInt("0xffffffffffffffff");
  for (let i = 0; i < str.length; i++) {
    h ^= BigInt(str.charCodeAt(i));
    h = (h * prime) & mask;
  }
  return h & BigInt("0x0fffffff");
}

/** Build a structured key: (prefix << 56) | (domain28 << 28) | item28 */
function buildStructuredKey(prefix: number, domain: string, item: string): string {
  const p = BigInt(prefix) << 56n;
  const d = fnv28(domain) << 28n;
  const it = fnv28(item);
  return String(p | d | it);
}

export function NewRecordDialog({
  open, onClose, onSave, onLookup, onNotice,
}: Props) {
  const [keyMode, setKeyMode] = useState<"raw" | "builder" | "hash">("builder");
  const [key, setKey] = useState("");
  const [seed, setSeed] = useState("");
  const [prefix, setPrefix] = useState<number>(1);
  const [domain, setDomain] = useState("");
  const [itemId, setItemId] = useState("");
  const [mode, setMode] = useState("raw");
  const [raw, setRaw] = useState("");
  const [fields, setFields] = useState<Field[]>([
    { id: "initial", name: "", type: "string", value: "" },
  ]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const partitions = getRegisteredPartitions();

  const resolvedKey = (): string => {
    if (keyMode === "raw") return key;
    if (keyMode === "hash") return fnv1a64(seed.trim());
    return buildStructuredKey(prefix, domain, itemId);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    const finalKey = resolvedKey();
    if (!/^(0|[1-9]\d*)$/.test(finalKey) || BigInt(finalKey) > 18446744073709551615n) {
      setError("Key is not a valid uint64 decimal.");
      return;
    }
    let value: string;
    try {
      value = mode === "builder" ? buildFields(fields) : raw;
    } catch (e) {
      setError((e as Error).message);
      return;
    }
    if (!value.length) { setError("Enter a value to save."); return; }
    setBusy(true);
    try {
      try {
        await onLookup(finalKey);
        setError("This key already exists. Choose another to avoid overwriting.");
        return;
      } catch (lookupError) {
        if ((lookupError as Error).message !== "That record was not found.") throw lookupError;
      }
      await onSave(finalKey, value);
      onNotice("Record created.");
      setKey(""); setRaw(""); setSeed(""); setDomain(""); setItemId("");
      onClose();
    } catch (saveError) {
      setError((saveError as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const previewKey = (() => {
    try {
      const k = resolvedKey();
      if (!k) return null;
      if (!/^(0|[1-9]\d*)$/.test(k)) return null;
      return k;
    } catch { return null; }
  })();

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v && !busy) onClose(); }}>
      <DialogContent>
        <DialogTitle className="pr-8 text-xl font-semibold">New Record</DialogTitle>
        <DialogDescription className="mt-1 text-sm text-muted-foreground">
          Add a key–value pair to BeastDB. Keys must be unique uint64 values.
        </DialogDescription>
        <form onSubmit={submit} className="mt-5 space-y-5">
          {/* ── Key Section ────────────────────────────────────── */}
          <div className="space-y-2">
            <p className="text-xs font-mono font-semibold uppercase tracking-wider text-zinc-400">64-bit Key</p>
            <div className="grid grid-cols-3 rounded-lg border border-zinc-800 bg-zinc-900/80 p-1 text-xs">
              {(["builder", "hash", "raw"] as const).map((m) => (
                <button key={m} type="button" onClick={() => setKeyMode(m)}
                  className={`rounded-md py-1.5 capitalize transition ${keyMode === m ? "bg-beast-lime/15 text-beast-lime border border-beast-lime/30 font-medium" : "text-zinc-400"}`}>
                  {m === "builder" ? "Partition Builder" : m === "hash" ? "Hash Seed" : "Raw Decimal"}
                </button>
              ))}
            </div>

            {keyMode === "builder" && (
              <div className="space-y-2">
                <select value={prefix} onChange={(e) => setPrefix(Number(e.target.value))}
                  className="w-full rounded-md border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm font-mono text-white focus:outline-none focus:ring-1 focus:ring-beast-lime">
                  {partitions.map((p) => (
                    <option key={p.prefix} value={p.prefix}>
                      0x{p.prefix.toString(16).padStart(2, "0").toUpperCase()} · {p.label}
                    </option>
                  ))}
                </select>
                <div className="grid grid-cols-2 gap-2">
                  <Input placeholder="Domain / user hash seed" value={domain} onChange={(e) => setDomain(e.target.value)} aria-label="Domain seed" />
                  <Input placeholder="Item ID / record seed" value={itemId} onChange={(e) => setItemId(e.target.value)} aria-label="Item seed" />
                </div>
                <p className="text-[11px] text-zinc-500 font-mono">
                  key = (prefix &lt;&lt; 56) | (FNV28(domain) &lt;&lt; 28) | FNV28(item)
                </p>
              </div>
            )}

            {keyMode === "hash" && (
              <div className="flex gap-2">
                <Input value={seed} onChange={(e) => setSeed(e.target.value)} placeholder="Identifier to hash with FNV-1a 64-bit" aria-label="Hash seed" />
              </div>
            )}

            {keyMode === "raw" && (
              <Input inputMode="numeric" value={key} onChange={(e) => setKey(e.target.value)} placeholder="e.g. 72057594037927936" />
            )}

            {previewKey && (
              <div className="flex items-center gap-2 rounded-md border border-beast-lime/20 bg-beast-lime/5 px-3 py-2 text-xs font-mono">
                <Wand2 size={12} className="text-beast-lime shrink-0" />
                <span className="text-zinc-400">Key preview:</span>
                <span className="text-beast-lime">{previewKey}</span>
              </div>
            )}
          </div>

          {/* ── Value Section ───────────────────────────────────── */}
          <Tabs.Root value={mode} onValueChange={setMode}>
            <Tabs.List aria-label="Value editor"
              className="grid grid-cols-2 rounded-lg border border-zinc-800 bg-zinc-900/80 p-1 text-xs">
              {["raw", "builder"].map((m) => (
                <Tabs.Trigger key={m} value={m}
                  className="rounded-md py-1.5 transition data-[state=active]:bg-beast-lime/15 data-[state=active]:text-beast-lime data-[state=active]:border data-[state=active]:border-beast-lime/30 data-[state=active]:font-medium text-zinc-400">
                  {m === "raw" ? "Raw JSON / Payload" : "Structured Builder"}
                </Tabs.Trigger>
              ))}
            </Tabs.List>
            <Tabs.Content value="raw" className="mt-3">
              <label htmlFor="record-value" className="text-sm font-medium">Value · JSON, text, or token</label>
              <textarea id="record-value" rows={7} value={raw} onChange={(e) => setRaw(e.target.value)}
                className="mt-2 w-full rounded-md border border-zinc-800 bg-zinc-900 p-3 font-mono text-sm focus:outline-none focus:ring-1 focus:ring-beast-lime"
                placeholder='{"id": 1, "name": "beast"}' />
            </Tabs.Content>
            <Tabs.Content value="builder">
              <RecordFields fields={fields} setFields={setFields} />
            </Tabs.Content>
          </Tabs.Root>

          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}

          <div className="flex justify-end gap-2 border-t border-zinc-800 pt-4">
            <Button type="button" variant="outline" onClick={onClose} disabled={busy}>Cancel</Button>
            <Button type="submit" disabled={busy}>{busy ? "Writing to WAL…" : "Create Record"}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
