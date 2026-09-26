"use client";
import { Copy } from "lucide-react";
import { Button } from "./ui/button";

interface Props {
  keyStr: string;
  prefixLabel?: string;
  userHash?: number | null;
  itemHash?: number | null;
  onNotice?: (msg: string, err?: boolean) => void;
}

export function KeyBitSlicer({ keyStr, prefixLabel, userHash, itemHash, onNotice }: Props) {
  let bits = "";
  try {
    bits = BigInt(keyStr).toString(2).padStart(64, "0");
  } catch {
    return null;
  }

  const prefixBits = bits.slice(0, 8);
  const userBits = bits.slice(8, 36);
  const itemBits = bits.slice(36, 64);
  const prefixHex = "0x" + Number(BigInt("0b" + prefixBits)).toString(16).padStart(2, "0").toUpperCase();

  const copyBits = async () => {
    try {
      await navigator.clipboard.writeText(bits);
      onNotice?.("64-bit binary copied.");
    } catch {
      onNotice?.("Copy failed.", true);
    }
  };

  return (
    <div className="space-y-3 rounded-lg border border-zinc-800/80 bg-zinc-950/60 p-3.5">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-semibold font-mono uppercase tracking-wider text-zinc-400">
          64-Bit Keyspace Anatomy
        </span>
        <Button variant="ghost" size="icon" className="h-6 w-6 text-zinc-400 hover:text-white" onClick={() => void copyBits()} title="Copy 64-bit binary">
          <Copy size={12} />
        </Button>
      </div>

      {/* Visual Segmented Bit Bar */}
      <div className="flex h-3 w-full overflow-hidden rounded bg-zinc-900 border border-zinc-800">
        <div className="h-full bg-beast-lime transition-all" style={{ width: "12.5%" }} title={`Partition Prefix: 8 bits (${prefixBits})`} />
        <div className="h-full bg-cyan-400 transition-all" style={{ width: "43.75%" }} title={`Domain/User Hash: 28 bits (${userBits})`} />
        <div className="h-full bg-purple-400 transition-all" style={{ width: "43.75%" }} title={`Item/Sequence: 28 bits (${itemBits})`} />
      </div>

      {/* Legend & Values */}
      <div className="grid grid-cols-3 gap-2 text-[11px]">
        <div className="rounded border border-zinc-800/60 bg-zinc-900/50 p-2">
          <div className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-beast-lime" />
            <span className="text-zinc-400 font-mono text-[10px]">PREFIX (8b)</span>
          </div>
          <p className="mt-1 font-mono font-medium text-beast-lime truncate">{prefixHex} {prefixLabel ? `· ${prefixLabel}` : ""}</p>
        </div>
        <div className="rounded border border-zinc-800/60 bg-zinc-900/50 p-2">
          <div className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-cyan-400" />
            <span className="text-zinc-400 font-mono text-[10px]">DOMAIN (28b)</span>
          </div>
          <p className="mt-1 font-mono font-medium text-cyan-300 truncate">{userHash != null ? userHash : "—"}</p>
        </div>
        <div className="rounded border border-zinc-800/60 bg-zinc-900/50 p-2">
          <div className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-purple-400" />
            <span className="text-zinc-400 font-mono text-[10px]">ITEM / SEQ (28b)</span>
          </div>
          <p className="mt-1 font-mono font-medium text-purple-300 truncate">{itemHash != null ? itemHash : "—"}</p>
        </div>
      </div>
    </div>
  );
}
