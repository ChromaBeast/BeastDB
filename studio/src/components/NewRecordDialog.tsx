"use client";
import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { fnv1a64 } from "../utils/format";
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
export function NewRecordDialog({
  open,
  onClose,
  onSave,
  onLookup,
  onNotice,
}: Props) {
  const [key, setKey] = useState("");
  const [seed, setSeed] = useState("");
  const [mode, setMode] = useState("raw");
  const [raw, setRaw] = useState("");
  const [fields, setFields] = useState<Field[]>([
    { id: "initial", name: "", type: "string", value: "" },
  ]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    if (!/^(0|[1-9]\d*)$/.test(key) || BigInt(key) > 18446744073709551615n) {
      setError("Enter a decimal key from 0 to 18446744073709551615.");
      return;
    }
    let value: string;
    try {
      value = mode === "builder" ? buildFields(fields) : raw;
    } catch (e) {
      setError((e as Error).message);
      return;
    }
    if (!value.length) {
      setError("Enter a value to save.");
      return;
    }
    setBusy(true);
    try {
      try {
        await onLookup(key);
        setError(
          "This key already exists. Choose another key to avoid replacing its value.",
        );
        return;
      } catch (lookupError) {
        if ((lookupError as Error).message !== "That record was not found.")
          throw lookupError;
      }
      await onSave(key, value);
      onNotice("Record created.");
      setKey("");
      setRaw("");
      onClose();
    } catch (saveError) {
      setError((saveError as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v && !busy) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle className="pr-8 text-xl font-semibold">
          New record
        </DialogTitle>
        <DialogDescription className="mt-1 text-sm text-muted-foreground">
          Add a key and value to BeastDB. The key must be unique.
        </DialogDescription>
        <form onSubmit={submit} className="mt-5 space-y-5">
          <div className="space-y-2">
            <label htmlFor="record-key" className="text-sm font-medium">
              64-bit decimal key
            </label>
            <Input
              id="record-key"
              inputMode="numeric"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="e.g. 18446744073709551615"
              required
            />
            <div className="flex gap-2">
              <Input
                aria-label="Identifier to hash"
                value={seed}
                onChange={(e) => setSeed(e.target.value)}
                placeholder="Or enter an identifier to hash"
              />
              <Button
                type="button"
                variant="outline"
                disabled={!seed.trim()}
                onClick={() => setKey(fnv1a64(seed.trim()))}
              >
                Use hash
              </Button>
            </div>
          </div>
          <Tabs.Root value={mode} onValueChange={setMode}>
            <Tabs.List
              aria-label="Value editor"
              className="grid grid-cols-2 rounded-lg border border-zinc-800 bg-zinc-900/80 p-1 text-xs"
            >
              <Tabs.Trigger
                value="raw"
                className="rounded-md py-1.5 transition data-[state=active]:bg-beast-lime/15 data-[state=active]:text-beast-lime data-[state=active]:border data-[state=active]:border-beast-lime/30 data-[state=active]:font-medium text-zinc-400"
              >
                Raw JSON / Payload
              </Tabs.Trigger>
              <Tabs.Trigger
                value="builder"
                className="rounded-md py-1.5 transition data-[state=active]:bg-beast-lime/15 data-[state=active]:text-beast-lime data-[state=active]:border data-[state=active]:border-beast-lime/30 data-[state=active]:font-medium text-zinc-400"
              >
                Structured Builder
              </Tabs.Trigger>
            </Tabs.List>
            <Tabs.Content value="raw" className="mt-3">
              <label htmlFor="record-value" className="text-sm font-medium">
                Value · JSON, text, or token
              </label>
              <textarea
                id="record-value"
                rows={8}
                value={raw}
                onChange={(e) => setRaw(e.target.value)}
                className="mt-2 w-full rounded-md border bg-card p-3 font-mono text-sm"
                placeholder="Enter the exact value to store"
              />
            </Tabs.Content>
            <Tabs.Content value="builder">
              <RecordFields fields={fields} setFields={setFields} />
            </Tabs.Content>
          </Tabs.Root>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2 border-t pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Create record"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
