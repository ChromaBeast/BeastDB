"use client";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "./ui/button";

interface Props {
  page: number;
  pageSize: number;
  totalShown: number;
  hasMore?: boolean;
  loadingMore?: boolean;
  onPageChange: (newPage: number) => void;
  onLoadMore?: () => void;
}

export function PaginationBar({
  page,
  pageSize,
  totalShown,
  hasMore,
  loadingMore,
  onPageChange,
  onLoadMore,
}: Props) {
  const totalPages = Math.max(1, Math.ceil(totalShown / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  const startIdx = totalShown === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const endIdx = Math.min(safePage * pageSize, totalShown);

  return (
    <nav
      aria-label="Pagination Navigation"
      className="flex flex-wrap items-center justify-between gap-3 border-t border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground backdrop-blur-sm"
    >
      <div className="flex items-center gap-2">
        <span className="font-medium text-foreground">
          Showing {startIdx}–{endIdx} of {totalShown} loaded
        </span>
        <span className="text-[11px] text-muted-foreground/80">
          {hasMore ? "(more available in partition)" : "(all loaded)"}
        </span>
      </div>

      <div className="flex items-center gap-1.5">
        {hasMore && onLoadMore && safePage === totalPages && (
          <Button
            variant="outline"
            size="sm"
            disabled={loadingMore}
            onClick={onLoadMore}
            className="mr-2 h-7 px-2.5 text-xs font-medium"
            aria-label="Fetch next page chunk from database engine"
          >
            {loadingMore ? "Loading…" : "Fetch more records"}
          </Button>
        )}

        <Button
          variant="outline"
          size="icon"
          disabled={safePage <= 1}
          onClick={() => onPageChange(safePage - 1)}
          className="h-7 w-7"
          aria-label="Previous page"
        >
          <ChevronLeft size={14} />
        </Button>

        <span className="px-2 text-xs font-mono font-medium text-foreground" aria-current="page">
          {safePage} / {totalPages}
        </span>

        <Button
          variant="outline"
          size="icon"
          disabled={safePage >= totalPages}
          onClick={() => onPageChange(safePage + 1)}
          className="h-7 w-7"
          aria-label="Next page"
        >
          <ChevronRight size={14} />
        </Button>
      </div>
    </nav>
  );
}
