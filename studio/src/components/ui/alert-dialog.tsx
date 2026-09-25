"use client";
import * as React from "react";
import * as Alert from "@radix-ui/react-alert-dialog";
import { cn } from "@/lib/utils";
export const AlertDialog = Alert.Root;
export const AlertDialogTitle = Alert.Title;
export const AlertDialogDescription = Alert.Description;
export const AlertDialogCancel = Alert.Cancel;
export const AlertDialogAction = Alert.Action;
export function AlertDialogContent({
  children,
  className,
  ...props
}: React.ComponentPropsWithoutRef<typeof Alert.Content>) {
  return (
    <Alert.Portal>
      <Alert.Overlay className="fixed inset-0 z-[60] bg-black/55" />
      <Alert.Content
        className={cn(
          "fixed left-1/2 top-1/2 z-[60] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border bg-card p-6 text-card-foreground shadow-xl",
          className,
        )}
        {...props}
      >
        {children}
      </Alert.Content>
    </Alert.Portal>
  );
}
