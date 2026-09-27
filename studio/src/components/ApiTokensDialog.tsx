"use client";
import { useEffect, useState } from "react";
import { Check, Copy, Key, Plus, Trash2, X } from "lucide-react";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Button } from "./ui/button";
import { APITokenItem } from "../types";

interface Props {
  open: boolean;
  onClose: () => void;
  onNotice: (msg: string, err?: boolean) => void;
}

export function ApiTokensDialog({ open, onClose, onNotice }: Props) {
  const [tokens, setTokens] = useState<APITokenItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [role, setRole] = useState("admin");
  const [newSecret, setNewSecret] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const fetchTokens = async () => {
    setLoading(true);
    try {
      const res = await fetch("/api/tokens");
      if (res.ok) {
        const json = await res.json();
        setTokens(json.tokens || []);
      }
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      void fetchTokens();
      setNewSecret(null);
      setName("");
    }
  }, [open]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      const res = await fetch("/api/tokens", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name.trim(), role }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      setNewSecret(data.token);
      setName("");
      setCreating(false);
      onNotice("API token generated and saved in partition 0x05.");
      await fetchTokens();
    } catch (err) {
      onNotice((err as Error).message, true);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      const res = await fetch(`/api/tokens?id=${encodeURIComponent(id)}`, { method: "DELETE" });
      if (!res.ok) throw new Error(await res.text());
      onNotice("Token revoked.");
      await fetchTokens();
    } catch (err) {
      onNotice((err as Error).message, true);
    }
  };

  const copyToClipboard = () => {
    if (!newSecret) return;
    navigator.clipboard.writeText(newSecret);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
    onNotice("Token copied to clipboard!");
  };

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-2xl bg-zinc-950 border-zinc-800 text-foreground p-6">
        <div className="flex flex-row items-center justify-between pb-4 border-b border-zinc-800">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-400">
              <Key size={18} />
            </div>
            <div>
              <DialogTitle className="text-base font-semibold text-white">API Access Tokens</DialogTitle>
              <p className="text-xs text-zinc-400 mt-0.5">Persistent database bearer tokens (Partition 0x05)</p>
            </div>
          </div>
          {!creating && (
            <Button size="sm" onClick={() => setCreating(true)} className="bg-beast-lime text-black hover:bg-beast-lime-hover font-medium text-xs">
              <Plus size={14} className="mr-1" /> New Token
            </Button>
          )}
        </div>

        {newSecret && (
          <div className="my-3 p-4 rounded-lg bg-beast-lime/10 border border-beast-lime/30 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-beast-lime">Copy Your New Token</span>
              <span className="text-[11px] text-zinc-400">You won&apos;t be able to see it again!</span>
            </div>
            <div className="flex items-center gap-2 bg-black/60 rounded px-3 py-2 border border-zinc-800">
              <code className="text-xs font-mono text-white flex-1 truncate">{newSecret}</code>
              <Button size="sm" variant="ghost" onClick={copyToClipboard} className="h-7 px-2 text-beast-lime hover:bg-beast-lime/20">
                {copied ? <Check size={14} /> : <Copy size={14} />}
              </Button>
            </div>
          </div>
        )}

        {creating && (
          <form onSubmit={handleCreate} className="my-3 p-4 rounded-lg bg-zinc-900/60 border border-zinc-800 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-white">Generate API Token</span>
              <button type="button" onClick={() => setCreating(false)} className="text-zinc-500 hover:text-white"><X size={14} /></button>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <input
                type="text"
                placeholder="Token name (e.g. Unfinished Backend)"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="col-span-2 rounded bg-black px-3 py-1.5 text-xs text-white border border-zinc-800 focus:outline-none focus:border-beast-lime"
                autoFocus
              />
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                className="rounded bg-black px-2 py-1.5 text-xs text-white border border-zinc-800 focus:outline-none focus:border-beast-lime"
              >
                <option value="admin">Admin</option>
                <option value="viewer">Viewer</option>
              </select>
            </div>
            <div className="flex justify-end gap-2 pt-1">
              <Button size="sm" variant="outline" type="button" onClick={() => setCreating(false)} className="text-xs h-7">Cancel</Button>
              <Button size="sm" type="submit" className="bg-beast-lime text-black hover:bg-beast-lime-hover font-medium text-xs h-7">Create</Button>
            </div>
          </form>
        )}

        <div className="mt-2 space-y-2 max-h-72 overflow-y-auto pr-1">
          {loading ? (
            <p className="py-6 text-center text-xs text-zinc-500 font-mono">Loading active tokens...</p>
          ) : tokens.length === 0 ? (
            <div className="py-8 text-center border border-dashed border-zinc-800/80 rounded-lg">
              <Key size={24} className="mx-auto text-zinc-600 mb-2" />
              <p className="text-xs font-medium text-zinc-400">No API tokens configured</p>
              <p className="text-[11px] text-zinc-500 mt-1 max-w-sm mx-auto">Generate a token to allow services like Unfinished or scripts to connect securely via gRPC.</p>
            </div>
          ) : (
            tokens.map((t) => (
              <div key={t.id} className="flex items-center justify-between p-3 rounded-lg bg-zinc-900/40 border border-zinc-800/80 hover:border-zinc-700 transition">
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold text-white">{t.name}</span>
                    <span className={`text-[10px] font-mono px-1.5 py-0.2 rounded border ${t.role === "admin" ? "bg-beast-lime/10 text-beast-lime border-beast-lime/30" : "bg-zinc-800 text-zinc-300 border-zinc-700"}`}>{t.role}</span>
                  </div>
                  <div className="flex items-center gap-2 text-[11px] font-mono text-zinc-400">
                    <code>{t.masked_token}</code>
                    <span>•</span>
                    <span>{new Date(t.created_at).toLocaleDateString()}</span>
                  </div>
                </div>
                <Button size="sm" variant="ghost" onClick={() => handleDelete(t.id)} title="Revoke Token" className="text-zinc-500 hover:text-red-400 hover:bg-red-500/10 h-7 w-7 p-0">
                  <Trash2 size={13} />
                </Button>
              </div>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
