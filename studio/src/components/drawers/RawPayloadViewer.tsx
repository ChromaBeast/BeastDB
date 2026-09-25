"use client";

import React, { useState, useMemo } from "react";
import { Copy, Check, Search, Download } from "lucide-react";
import { formatBytes } from "../../utils/format";

interface RawPayloadViewerProps {
  raw: string;
  byteSize: number;
}

export const RawPayloadViewer: React.FC<RawPayloadViewerProps> = ({ raw, byteSize }) => {
  const [copied, setCopied] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");

  const formattedText = useMemo(() => {
    try {
      return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
      return raw;
    }
  }, [raw]);

  const handleCopy = () => {
    navigator.clipboard.writeText(formattedText);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownload = () => {
    const blob = new Blob([formattedText], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `payload-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const lines = useMemo(() => formattedText.split("\n"), [formattedText]);

  return (
    <div className="flex flex-col space-y-3">
      {/* Viewer Toolbar */}
      <div className="flex items-center justify-between gap-2 rounded-xl border border-white/[0.08] bg-black/40 p-2 text-xs">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search payload..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full rounded-lg border border-white/10 bg-black/60 py-1 pl-8 pr-2 text-xs text-white placeholder-slate-500 outline-none focus:border-purple-500"
          />
        </div>

        <div className="flex items-center gap-2">
          <span className="font-mono text-[11px] text-slate-400">{formatBytes(byteSize)}</span>
          <button
            onClick={handleCopy}
            className="flex items-center gap-1 rounded-lg border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[11px] text-slate-300 hover:text-white"
          >
            {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
            <span>{copied ? "Copied" : "Copy"}</span>
          </button>
          <button
            onClick={handleDownload}
            className="rounded-lg border border-white/10 bg-white/[0.04] p-1 text-slate-300 hover:text-white"
            title="Download payload"
          >
            <Download className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* Code Viewer */}
      <div className="rounded-xl border border-white/[0.08] bg-black/60 p-4 overflow-x-auto max-h-[550px]">
        <pre className="font-mono text-xs leading-relaxed text-purple-200/90 whitespace-pre">
          {searchTerm.trim()
            ? lines.map((line, idx) => {
                const match = line.toLowerCase().includes(searchTerm.toLowerCase());
                return (
                  <div
                    key={idx}
                    className={match ? "bg-purple-600/30 text-white rounded px-1" : ""}
                  >
                    {line}
                  </div>
                );
              })
            : formattedText}
        </pre>
      </div>
    </div>
  );
};
