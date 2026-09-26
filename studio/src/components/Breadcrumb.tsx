import { ChevronRight } from "lucide-react";

interface Props {
  partitionLabel: string;
  recordLabel?: string;
  onBack: () => void;
}

export function Breadcrumb({ partitionLabel, recordLabel, onBack }: Props) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-xs text-muted-foreground">
      <span className="font-medium text-foreground">BeastDB</span>
      <ChevronRight size={12} />
      <button
        type="button"
        onClick={onBack}
        className="hover:text-foreground hover:underline"
      >
        {partitionLabel}
      </button>
      {recordLabel && (
        <>
          <ChevronRight size={12} />
          <span className="truncate font-medium text-foreground max-w-[180px]">{recordLabel}</span>
        </>
      )}
    </nav>
  );
}
