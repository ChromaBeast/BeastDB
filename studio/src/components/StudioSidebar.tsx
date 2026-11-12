"use client";
import {
  Activity,
  Database,
  Key,
  LogOut,
  Moon,
  RefreshCw,
  Sliders,
  Sun,
  type LucideIcon,
} from "lucide-react";
import { BeastDBLogo } from "./BeastDBLogo";
import { RoleBadge } from "./RoleBadge";
import { SessionUser } from "../types";
import { useTheme } from "../hooks/useTheme";
import { cn } from "@/lib/utils";

export type StudioView = "overview" | "explorer" | "operations";

interface Props {
  view: StudioView;
  setView: (view: StudioView) => void;
  user: SessionUser | null;
  busy: boolean;
  onRefresh: () => void;
  onOpenTokens: () => void;
}

const PAGES: { id: StudioView; label: string; icon: LucideIcon }[] = [
  { id: "overview", label: "Overview", icon: Activity },
  { id: "explorer", label: "Table Editor", icon: Database },
  { id: "operations", label: "Operations", icon: Sliders },
];

const itemBase =
  "flex w-full shrink-0 items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors";

export function StudioSidebar({ view, setView, user, busy, onRefresh, onOpenTokens }: Props) {
  const theme = useTheme();
  return (
    <aside
      className="flex shrink-0 flex-col border-b border-border bg-sidebar md:h-dvh md:w-60 md:border-b-0 md:border-r"
    >
      <div className="flex items-center justify-between px-4 py-3 md:px-5 md:pb-2 md:pt-5">
        <BeastDBLogo />
        <div className="flex items-center gap-1 md:hidden">
          <IconButton label="Refresh data" onClick={onRefresh} disabled={busy}>
            <RefreshCw size={16} className={busy ? "animate-spin" : ""} />
          </IconButton>
          <IconButton label={theme.dark ? "Use light theme" : "Use dark theme"} onClick={theme.toggle}>
            {theme.dark ? <Sun size={16} /> : <Moon size={16} />}
          </IconButton>
        </div>
      </div>

      <nav
        aria-label="Main navigation"
        className="flex gap-1 overflow-x-auto px-3 pb-3 md:flex-1 md:flex-col md:overflow-y-auto md:px-3 md:pt-4"
      >
        {PAGES.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            aria-current={view === id ? "page" : undefined}
            onClick={() => setView(id)}
            className={cn(
              itemBase,
              view === id
                ? "bg-primary/10 font-medium text-primary"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
          >
            <Icon size={16} />
            {label}
          </button>
        ))}
        <button
          onClick={onOpenTokens}
          className={cn(itemBase, "text-muted-foreground hover:bg-muted hover:text-foreground")}
        >
          <Key size={16} />
          API Keys &amp; Roles
        </button>
      </nav>

      <div className="hidden items-center gap-2 border-t border-border p-3 md:flex">
        <div
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-beast-lime text-[11px] font-bold text-black"
          aria-hidden="true"
        >
          {(user?.username || "??").slice(0, 2).toUpperCase()}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-xs" title={user?.username || "Session unavailable"}>
            {user?.username || "Account"}
          </p>
          <RoleBadge role={user?.role} />
        </div>
        <IconButton label="Refresh data" onClick={onRefresh} disabled={busy}>
          <RefreshCw size={15} className={busy ? "animate-spin" : ""} />
        </IconButton>
        <IconButton label={theme.dark ? "Use light theme" : "Use dark theme"} onClick={theme.toggle}>
          {theme.dark ? <Sun size={15} /> : <Moon size={15} />}
        </IconButton>
        <form action="/logout" method="POST">
          <IconButton label="Sign out" type="submit">
            <LogOut size={15} />
          </IconButton>
        </form>
      </div>
    </aside>
  );
}

function IconButton({
  label,
  children,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { label: string }) {
  return (
    <button
      aria-label={label}
      title={label}
      className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
      {...props}
    >
      {children}
    </button>
  );
}
