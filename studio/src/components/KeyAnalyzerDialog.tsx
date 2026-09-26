"use client";
import { useState } from "react";
import { decodeKey } from "../utils/key-decoder";
import { fnv1a64 } from "../utils/format";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Badge } from "./ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "./ui/dialog";
import { KeyBitSlicer } from "./KeyBitSlicer";

interface Props {
  open: boolean;
  onClose: () => void;
}

const PRESETS = [
  { label: "User (0x01)", prefix: "72057594037927936" },
  { label: "Token (0x05)", prefix: "360287970189639680" },
  { label: "Catalog (0x10)", prefix: "1152921504606846976" },
];

export function KeyAnalyzerDialog({ open, onClose }: Props) {
  const [key, setKey] = useState("");
  const [seed, setSeed] = useState("");

  const valid = /^(0|[1-9]\d*)$/.test(key) && BigInt(key || "0") <= 18446744073709551615n;
  const decoded = valid && key ? decodeKey(key) : null;

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-xl">
        <DialogTitle className="text-xl font-bold tracking-tight">Key Analyzer</DialogTitle>
        <DialogDescription className="mt-1 text-xs text-muted-foreground">
          Inspect 64-bit BeastDB keys, decode partition anatomy, or hash identifiers via FNV-1a.
        </DialogDescription>

        <div className="mt-4 space-y-4">
          <div>
            <div className="flex items-center justify-between">
              <label htmlFor="analyzer-key" className="text-xs font-mono text-zinc-400">DECIMAL KEY</label>
              <div className="flex gap-1">
                {PRESETS.map((p) => (
                  <button key={p.prefix} type="button" onClick={() => setKey(p.prefix)} className="text-[10px] font-mono text-zinc-500 hover:text-beast-lime underline">
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <Input id="analyzer-key" className="mt-1.5 font-mono text-sm" value={key} onChange={(e) => setKey(e.target.value)} placeholder="e.g. 72057594037927936" />
          </div>

          <div>
            <label className="text-xs font-mono text-zinc-400">HASH STRING IDENTIFIER (FNV-1a)</label>
            <div className="mt-1.5 flex gap-2">
              <Input aria-label="Identifier to hash" value={seed} onChange={(e) => setSeed(e.target.value)} placeholder="e.g. alice@example.com or user_104" className="font-mono text-sm" />
              <Button variant="secondary" disabled={!seed.trim()} onClick={() => setKey(fnv1a64(seed.trim()))}>
                Hash
              </Button>
            </div>
          </div>

          {key && !valid && (
            <p role="alert" className="text-xs text-destructive font-mono">
              Invalid key: Must be an unsigned 64-bit integer (0 to 18446744073709551615).
            </p>
          )}

          {decoded && (
            <div className="space-y-3 pt-2">
              <KeyBitSlicer keyStr={decoded.raw} prefixLabel={decoded.prefixLabel} userHash={decoded.userHash} itemHash={decoded.itemHash} />
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded border border-zinc-800 bg-zinc-900/50 p-2.5">
                  <span className="text-[10px] font-mono text-zinc-400">HEXADECIMAL</span>
                  <p className="mt-1 font-mono text-white font-medium">{decoded.hex}</p>
                </div>
                <div className="rounded border border-zinc-800 bg-zinc-900/50 p-2.5">
                  <span className="text-[10px] font-mono text-zinc-400">PARTITION METADATA</span>
                  <div className="mt-1 flex items-center gap-1.5">
                    <Badge variant="lime">{decoded.prefixHex}</Badge>
                    <span className="truncate text-zinc-300 font-medium">{decoded.prefixLabel}</span>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
