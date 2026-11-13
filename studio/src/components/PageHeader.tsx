import type { ReactNode } from "react";

/** Shared page title block (Figma: Display/Page title + muted subtitle + right-aligned actions). */
export function PageHeader({ id, title, subtitle, actions }: { id: string; title: string; subtitle?: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 id={id} className="text-[22px] font-bold leading-tight tracking-tight text-foreground">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}
