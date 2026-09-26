export function RoleBadge({ role }: { role: string | undefined }) {
  if (role === "admin") {
    return (
      <span className="inline-flex items-center rounded-md border border-beast-lime/40 bg-beast-lime/15 px-2 py-0.5 text-[10px] font-mono font-semibold tracking-wider text-beast-lime">
        ADMIN
      </span>
    );
  }
  if (role === "viewer") {
    return (
      <span className="inline-flex items-center rounded-md border border-zinc-800 bg-zinc-900 px-2 py-0.5 text-[10px] font-mono font-semibold tracking-wider text-zinc-400">
        VIEWER
      </span>
    );
  }
  return null;
}
