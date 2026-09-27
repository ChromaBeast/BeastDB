import { ChevronRight, Database } from "lucide-react";

interface Props {
  partitionLabel: string;
  recordLabel?: string;
  onBack: () => void;
}

export function Breadcrumb({ partitionLabel, recordLabel, onBack }: Props) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-xs text-muted-foreground">
      <div className="flex items-center gap-1">
        <Database size={13} className="text-primary" />
        <span className="font-mono font-medium text-foreground">BeastDB</span>
      </div>
      <ChevronRight size={12} className="text-muted-foreground/60" />
      <button
        type="button"
        onClick={onBack}
        className="font-medium text-muted-foreground hover:text-foreground transition"
      >
        {partitionLabel}
      </button>
      {recordLabel && (
        <>
          <ChevronRight size={12} className="text-muted-foreground/60" />
          <span className="truncate font-mono font-medium text-foreground max-w-[200px]">{recordLabel}</span>
        </>
      )}
    </nav>
  );
}
