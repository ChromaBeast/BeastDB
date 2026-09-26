import * as React from "react";
import { cn } from "@/lib/utils";

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: "default" | "lime" | "secondary" | "outline" | "destructive" | "mono";
}

export function Badge({ className, variant = "default", ...props }: BadgeProps) {
  return (
    <div
      className={cn(
        "inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
        {
          "border-transparent bg-primary text-primary-foreground": variant === "default",
          "border-beast-lime/40 bg-beast-lime/15 text-beast-lime font-mono text-[10px] tracking-wide":
            variant === "lime",
          "border-transparent bg-secondary text-secondary-foreground": variant === "secondary",
          "border-border text-foreground": variant === "outline",
          "border-destructive/30 bg-destructive/15 text-destructive": variant === "destructive",
          "border-zinc-800 bg-zinc-900/90 text-zinc-400 font-mono text-[10px]": variant === "mono",
        },
        className,
      )}
      {...props}
    />
  );
}
