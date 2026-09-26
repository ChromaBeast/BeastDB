export function RoleBadge({ role }: { role: string | undefined }) {
  if (role === "admin") {
    return (
      <span className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">
        ADMIN
      </span>
    );
  }
  if (role === "viewer") {
    return (
      <span className="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600 dark:bg-slate-800 dark:text-slate-300">
        VIEWER
      </span>
    );
  }
  return null;
}
