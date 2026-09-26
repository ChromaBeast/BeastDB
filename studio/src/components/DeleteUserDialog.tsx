"use client";
import { AlertTriangle } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogTitle,
} from "./ui/alert-dialog";
import { Button } from "./ui/button";

interface Props {
  open: boolean;
  userHash: number | null;
  onConfirm: () => void;
  onClose: () => void;
}

export function DeleteUserDialog({ open, userHash, onConfirm, onClose }: Props) {
  return (
    <AlertDialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <AlertDialogContent className="border-red-950/50 bg-[#0c0c0e]">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-destructive/15 text-destructive">
            <AlertTriangle size={20} />
          </div>
          <div>
            <AlertDialogTitle className="text-base font-bold text-white">
              Cascade Delete User Account
            </AlertDialogTitle>
            <p className="font-mono text-xs text-zinc-400">Target User Hash: {userHash}</p>
          </div>
        </div>
        <AlertDialogDescription className="mt-3 text-xs leading-5 text-zinc-400">
          This operation purges the primary user credential record (`0x01`) and atomically seeks and deletes all associated records (profiles, tokens, relationships) across the entire keyspace.
        </AlertDialogDescription>
        <div className="mt-5 flex justify-end gap-2 border-t border-zinc-800/80 pt-3">
          <AlertDialogCancel asChild>
            <Button variant="outline" size="sm" onClick={onClose} className="text-xs">Cancel</Button>
          </AlertDialogCancel>
          <AlertDialogAction asChild>
            <Button variant="destructive" size="sm" onClick={onConfirm} className="text-xs">
              Purge User & Records
            </Button>
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialog>
  );
}
