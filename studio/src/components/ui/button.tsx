import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const styles = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-beast-lime text-black font-semibold shadow hover:bg-beast-lime-hover shadow-[0_0_15px_rgba(168,242,26,0.18)]",
        secondary: "border border-border bg-zinc-900 text-zinc-200 shadow-sm hover:bg-zinc-800 hover:text-white",
        outline: "border border-border bg-card shadow-sm hover:bg-zinc-800 hover:text-white",
        ghost: "hover:bg-zinc-800 hover:text-white",
        destructive:
          "bg-destructive text-destructive-foreground hover:opacity-90",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 px-3 text-xs",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);
export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof styles>;
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button
      ref={ref}
      className={cn(styles({ variant, size }), className)}
      {...props}
    />
  ),
);
Button.displayName = "Button";
