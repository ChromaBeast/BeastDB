export function RoleBadge({ role }: { role: string | undefined }) {
  if (role === "admin") {
    return (
      <span className="inline-flex items-center rounded-md border border-primary/40 bg-primary/10 px-2 py-0.5 text-[10px] font-mono font-semibold tracking-wider text-primary">
        ADMIN
      </span>
    );
  }
  if (role === "viewer") {
    return (
      <span className="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-[10px] font-mono font-semibold tracking-wider text-muted-foreground">
        VIEWER
      </span>
    );
  }
  return null;
}
