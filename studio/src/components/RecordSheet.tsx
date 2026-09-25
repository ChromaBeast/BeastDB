"use client";
import { Trash2 } from "lucide-react";
import { UniversalRecord } from "../types";
import { formatBytes } from "../utils/format";
import { Button } from "./ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "./ui/sheet";
import { RecordDetails } from "./RecordDetails";

interface Props {
  record: UniversalRecord | null;
  canWrite: boolean;
  onClose: () => void;
  onDelete: (key: string) => void;
  onNotice: (message: string, error?: boolean) => void;
}
export function RecordSheet({
  record,
  canWrite,
  onClose,
  onDelete,
  onNotice,
}: Props) {
  return (
    <Sheet
      open={Boolean(record)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent aria-describedby="record-description">
        {record ? (
          <>
            <div className="pr-8">
              <p className="text-xs uppercase tracking-widest text-muted-foreground">
                {record.prefixLabel}
              </p>
              <SheetTitle className="mt-1 truncate text-xl font-semibold">
                {record.primaryLabel}
              </SheetTitle>
              <SheetDescription
                id="record-description"
                className="mt-1 font-mono text-xs"
              >
                Key {record.keyStr} · {formatBytes(record.byteSize)}
              </SheetDescription>
            </div>
            <RecordDetails record={record} onNotice={onNotice} />
            {canWrite && (
              <div className="border-t pt-4">
                <Button
                  variant="outline"
                  className="text-destructive"
                  onClick={() => onDelete(record.keyStr)}
                >
                  <Trash2 size={15} />
                  Delete record
                </Button>
              </div>
            )}
          </>
        ) : (
          <SheetTitle>Record</SheetTitle>
        )}
      </SheetContent>
    </Sheet>
  );
}
