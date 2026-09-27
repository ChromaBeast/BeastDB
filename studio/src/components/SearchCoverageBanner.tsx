"use client";
import { CheckCircle2, ChevronRight, RefreshCw, X } from "lucide-react";
import { Button } from "./ui/button";

interface Props {
  query: string;
  count: number;
  scannedCount: number;
  hasMore: boolean;
  prefix?: number;
  prefixLabel?: string;
  loadingMore?: boolean;
  onContinueScan?: () => void;
  onClear: () => void;
}

export function SearchCoverageBanner({
  query,
  count,
  scannedCount,
  hasMore,
  prefix,
  prefixLabel,
  loadingMore,
  onContinueScan,
  onClear,
}: Props) {
  const scopeText = prefix !== undefined
    ? `Partition 0x${prefix.toString(16).padStart(2, "0").toUpperCase()}${prefixLabel ? ` (${prefixLabel})` : ""}`
    : "Entire Keyspace";

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-primary/30 bg-primary/5 px-4 py-2.5 text-xs text-foreground">
      <div className="flex items-center gap-2">
        <CheckCircle2 size={16} className="text-primary shrink-0" />
        <div>
          <span className="font-semibold text-foreground">
            {count} {count === 1 ? "match" : "matches"} found
          </span>
          <span className="text-muted-foreground"> for &ldquo;{query}&rdquo; · </span>
          <span className="font-mono text-foreground">
            {scannedCount.toLocaleString()} keys inspected in {scopeText}
          </span>
          {hasMore && (
            <span className="ml-1 text-amber-600 dark:text-amber-400">
              (More uninspected keys remain in database)
            </span>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2">
        {hasMore && onContinueScan && (
          <Button
            size="sm"
            variant="outline"
            onClick={onContinueScan}
            disabled={loadingMore}
            className="h-7 text-xs gap-1 border-primary/40 text-primary hover:bg-primary/10"
          >
            {loadingMore ? (
              <RefreshCw size={12} className="animate-spin" />
            ) : (
              <ChevronRight size={14} />
            )}
            Scan Next Chunk
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          onClick={onClear}
          className="h-7 text-xs gap-1 text-muted-foreground hover:text-foreground"
        >
          <X size={13} />
          Clear Results
        </Button>
      </div>
    </div>
  );
}
