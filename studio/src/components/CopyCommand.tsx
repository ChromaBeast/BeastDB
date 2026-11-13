"use client";
import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "./ui/button";

/** Monospace command block with a copy button. Reports failure honestly if the clipboard is unavailable. */
export function CopyCommand({ label, command, onNotice }: { label: string; command: string; onNotice: (message: string, error?: boolean) => void }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      onNotice("Could not access the clipboard. Select the command and copy it manually.", true);
    }
  };
  return (
    <div>
      <p className="mb-1.5 text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-start justify-between gap-2 rounded-md border border-border bg-background px-3 py-2.5">
        <code className="min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-xs text-foreground">{command}</code>
        <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0" onClick={() => void copy()} aria-label={`Copy ${label} command`}>
          {copied ? <Check size={14} className="text-success" /> : <Copy size={14} />}
        </Button>
      </div>
    </div>
  );
}
