import { cn } from "@/lib/utils";

const TONE: Record<string, string> = {
  admin: "border-warning/40 bg-warning/10 text-warning",
  writer: "border-info/40 bg-info/10 text-info",
  viewer: "border-border bg-muted text-muted-foreground",
  replication: "border-border bg-transparent text-muted-foreground",
};

/** Role chip for API tokens (admin/writer/viewer/replication). */
export function TokenRoleBadge({ role }: { role: string }) {
  return (
    <span className={cn("inline-flex items-center rounded-md border px-2 py-0.5 font-mono text-[10px] font-semibold uppercase tracking-wider", TONE[role] ?? TONE.viewer)}>
      {role}
    </span>
  );
}
