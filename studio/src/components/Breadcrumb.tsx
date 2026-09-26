import { ChevronRight, Database } from "lucide-react";

interface Props {
  partitionLabel: string;
  recordLabel?: string;
  onBack: () => void;
}

export function Breadcrumb({ partitionLabel, recordLabel, onBack }: Props) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-xs text-zinc-400">
      <div className="flex items-center gap-1 text-zinc-500">
        <Database size={13} className="text-beast-lime" />
        <span className="font-mono font-medium text-zinc-300">BeastDB</span>
      </div>
      <ChevronRight size={12} className="text-zinc-600" />
      <button
        type="button"
        onClick={onBack}
        className="font-medium text-zinc-300 hover:text-white transition"
      >
        {partitionLabel}
      </button>
      {recordLabel && (
        <>
          <ChevronRight size={12} className="text-zinc-600" />
          <span className="truncate font-mono font-medium text-white max-w-[200px]">{recordLabel}</span>
        </>
      )}
    </nav>
  );
}
