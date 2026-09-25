"use client";
import { useState } from "react";
import { decodeKey } from "../utils/key-decoder";
import { fnv1a64 } from "../utils/format";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "./ui/dialog";

export function KeyAnalyzerDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const [key, setKey] = useState("");
  const [seed, setSeed] = useState("");
  const valid =
    /^(0|[1-9]\d*)$/.test(key) && BigInt(key || "0") <= 18446744073709551615n;
  const decoded = valid ? decodeKey(key) : null;
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
    >
      <DialogContent>
        <DialogTitle className="text-xl font-semibold">
          Key analyzer
        </DialogTitle>
        <DialogDescription className="mt-1 text-sm text-muted-foreground">
          Inspect a 64-bit BeastDB key or derive one from an identifier.
        </DialogDescription>
        <div className="mt-5 space-y-4">
          <div>
            <label htmlFor="analyzer-key" className="text-sm font-medium">
              Decimal key
            </label>
            <Input
              id="analyzer-key"
              className="mt-2 font-mono"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="Enter a key"
            />
          </div>
          <div className="flex gap-2">
            <Input
              aria-label="Identifier to hash"
              value={seed}
              onChange={(e) => setSeed(e.target.value)}
              placeholder="String identifier"
            />
            <Button
              variant="outline"
              disabled={!seed.trim()}
              onClick={() => setKey(fnv1a64(seed.trim()))}
            >
              Hash
            </Button>
          </div>
          {key && !valid && (
            <p role="alert" className="text-sm text-destructive">
              Enter an unsigned 64-bit decimal key.
            </p>
          )}
          {decoded && (
            <dl className="divide-y rounded-md border px-4 text-sm">
              <div className="flex justify-between gap-3 py-3">
                <dt className="text-muted-foreground">Hexadecimal</dt>
                <dd className="font-mono">{decoded.hex}</dd>
              </div>
              <div className="flex justify-between gap-3 py-3">
                <dt className="text-muted-foreground">Partition</dt>
                <dd>
                  {decoded.prefixLabel} ({decoded.prefixHex})
                </dd>
              </div>
              <div className="flex justify-between gap-3 py-3">
                <dt className="text-muted-foreground">User hash</dt>
                <dd className="font-mono">{decoded.userHash}</dd>
              </div>
              <div className="flex justify-between gap-3 py-3">
                <dt className="text-muted-foreground">Item hash</dt>
                <dd className="font-mono">{decoded.itemHash}</dd>
              </div>
            </dl>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
