"use client";
import { useState } from "react";
import * as Tabs from "@radix-ui/react-tabs";
import { Copy, Download } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { RecordValue } from "./RecordValue";
export function RecordDetails({
  record,
  onNotice,
}: {
  record: UniversalRecord;
  onNotice: (message: string, error?: boolean) => void;
}) {
  const [search, setSearch] = useState("");
  const formatted = (() => {
    try {
      return JSON.stringify(JSON.parse(record.raw), null, 2);
    } catch {
      return record.raw;
    }
  })();
  const bits = BigInt(record.keyStr).toString(2).padStart(64, "0");
  const copy = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      onNotice(`${label} copied.`);
    } catch {
      onNotice("Copy failed.", true);
    }
  };
  const download = () => {
    const blob = new Blob([record.raw], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `record-${record.keyStr}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <Tabs.Root
      key={record.keyStr}
      defaultValue="fields"
      className="mt-6 flex min-h-0 flex-1 flex-col"
    >
      <Tabs.List
        aria-label="Record details"
        className="grid grid-cols-3 rounded-md bg-muted p-1 text-sm"
      >
        <Tabs.Trigger
          value="fields"
          className="rounded px-2 py-1.5 data-[state=active]:bg-card data-[state=active]:shadow-sm"
        >
          Fields
        </Tabs.Trigger>
        <Tabs.Trigger
          value="raw"
          className="rounded px-2 py-1.5 data-[state=active]:bg-card data-[state=active]:shadow-sm"
        >
          Raw value
        </Tabs.Trigger>
        <Tabs.Trigger
          value="key"
          className="rounded px-2 py-1.5 data-[state=active]:bg-card data-[state=active]:shadow-sm"
        >
          Key details
        </Tabs.Trigger>
      </Tabs.List>
      <div className="mt-5 min-h-0 flex-1 overflow-y-auto pb-4">
        <Tabs.Content value="fields" className="space-y-3">
          <p className="text-xs text-muted-foreground">
            {record.format.replace("_", " ")} · {formatBytes(record.byteSize)}
          </p>
          {record.fields ? (
            Object.entries(record.fields).map(([k, v]) => (
              <div key={k} className="rounded-md border p-3">
                <div className="mb-2 font-mono text-xs font-medium text-muted-foreground">
                  {k}
                </div>
                <RecordValue value={v} />
              </div>
            ))
          ) : record.arrayItems ? (
            record.arrayItems.map((v, i) => (
              <div key={i} className="rounded-md border p-3">
                <div className="mb-2 font-mono text-xs text-muted-foreground">
                  [{i}]
                </div>
                <RecordValue value={v} />
              </div>
            ))
          ) : (
            <div className="rounded-md border p-3 text-sm">
              <RecordValue value={record.raw || "Empty value"} />
            </div>
          )}
        </Tabs.Content>
        <Tabs.Content value="raw" className="space-y-3">
          <div className="flex flex-wrap gap-2">
            <Input
              aria-label="Find in raw value"
              placeholder="Find in value"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="min-w-32 flex-1"
            />
            <Button
              variant="outline"
              size="icon"
              aria-label="Copy raw value"
              onClick={() => void copy(record.raw, "Raw value")}
            >
              <Copy size={15} />
            </Button>
            <Button
              variant="outline"
              size="icon"
              aria-label="Download raw value"
              onClick={download}
            >
              <Download size={15} />
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {search
              ? `${formatted.toLowerCase().split(search.toLowerCase()).length - 1} matches`
              : `${formatBytes(record.byteSize)} · original value preserved`}
          </p>
          <pre className="max-h-[65vh] overflow-auto rounded-md border bg-muted/50 p-4 text-xs leading-6 whitespace-pre-wrap break-all">
            {formatted}
          </pre>
        </Tabs.Content>
        <Tabs.Content value="key" className="space-y-4">
          <div className="rounded-md border p-4">
            <p className="text-xs text-muted-foreground">Decimal key</p>
            <div className="mt-1 flex items-center justify-between gap-2">
              <code className="break-all text-sm">{record.keyStr}</code>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Copy key"
                onClick={() => void copy(record.keyStr, "Key")}
              >
                <Copy size={15} />
              </Button>
            </div>
          </div>
          <div className="rounded-md border p-4">
            <p className="text-xs text-muted-foreground">Hexadecimal</p>
            <code className="mt-1 block break-all text-sm">
              {record.keyHex}
            </code>
          </div>
          <div className="rounded-md border p-4">
            <p className="text-xs text-muted-foreground">
              64-bit layout · 8-bit partition / 28-bit user / 28-bit item
            </p>
            <code className="mt-3 block break-all text-xs leading-6">
              {bits.slice(0, 8)}{" "}
              <span className="text-muted-foreground">·</span>{" "}
              {bits.slice(8, 36)}{" "}
              <span className="text-muted-foreground">·</span> {bits.slice(36)}
            </code>
            <dl className="mt-4 space-y-2 text-sm">
              <div className="flex justify-between">
                <dt>Partition</dt>
                <dd>
                  {record.prefixLabel} (0x
                  {record.prefix.toString(16).padStart(2, "0").toUpperCase()})
                </dd>
              </div>
              <div className="flex justify-between">
                <dt>User hash</dt>
                <dd className="font-mono">{record.userHash}</dd>
              </div>
              <div className="flex justify-between">
                <dt>Item hash</dt>
                <dd className="font-mono">{record.itemHash}</dd>
              </div>
            </dl>
          </div>
        </Tabs.Content>
      </div>
    </Tabs.Root>
  );
}
