"use client";
import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;

interface Props {
  onCreate: (name: string) => Promise<void>;
  onNotice: (message: string, error?: boolean) => void;
}

// Admin-only inline form: names a new collection; the server picks its key prefix.
export function NewCollectionForm({ onCreate, onNotice }: Props) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const valid = NAME_RE.test(name);

  const submit = async () => {
    if (!valid || busy) return;
    setBusy(true);
    try {
      await onCreate(name);
      onNotice(`Collection "${name}" created.`);
      setName("");
      setOpen(false);
    } catch (e) {
      onNotice((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <Button variant="ghost" size="sm" className="w-full justify-start" onClick={() => setOpen(true)}>
        <Plus size={14} /> New collection
      </Button>
    );
  }
  return (
    <form className="space-y-2 px-1" onSubmit={(e) => { e.preventDefault(); void submit(); }}>
      <Input
        aria-label="Collection name"
        autoFocus
        placeholder="e.g. orders"
        value={name}
        onChange={(e) => setName(e.target.value.toLowerCase())}
      />
      <p className="text-xs text-muted-foreground">
        {name && !valid ? "Use a-z, 0-9, - and _ (max 64)." : "A key prefix is assigned automatically."}
      </p>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={!valid || busy}>{busy ? "Creating…" : "Create"}</Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => { setOpen(false); setName(""); }}>Cancel</Button>
      </div>
    </form>
  );
}
