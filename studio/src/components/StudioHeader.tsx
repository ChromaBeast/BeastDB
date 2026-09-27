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
} from "lucide-react";
import { Button } from "./ui/button";
import { BeastDBLogo } from "./BeastDBLogo";
import { SessionUser } from "../types";
import { useTheme } from "../hooks/useTheme";
import { RoleBadge } from "./RoleBadge";

export type StudioView = "overview" | "explorer" | "operations";

interface Props {
  view: StudioView;
  setView: (view: StudioView) => void;
  user: SessionUser | null;
  busy: boolean;
  onRefresh: () => void;
  onOpenTokens?: () => void;
}
export function StudioHeader({ view, setView, user, busy, onRefresh, onOpenTokens }: Props) {
  const theme = useTheme();
  return (
    <header className="sticky top-0 z-40 shrink-0 border-b border-border bg-background/90 backdrop-blur-md">
      <div className="mx-auto flex max-w-[1600px] flex-wrap items-center gap-3 px-4 py-3 md:px-8">
        <div className="mr-auto flex items-center gap-3">
          <BeastDBLogo />
        </div>
        <nav
          aria-label="Main navigation"
          className="order-3 flex w-full gap-1 rounded-lg border border-border bg-muted/60 p-1 md:order-none md:w-auto"
        >
          <button
            aria-current={view === "overview" ? "page" : undefined}
            onClick={() => setView("overview")}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-3.5 py-1.5 text-xs transition md:flex-none ${view === "overview" ? "bg-primary/10 text-primary font-medium border border-primary/25 shadow-sm" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
          >
            <Activity size={14} />
            Overview
          </button>
          <button
            aria-current={view === "explorer" ? "page" : undefined}
            onClick={() => setView("explorer")}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-3.5 py-1.5 text-xs transition md:flex-none ${view === "explorer" ? "bg-primary/10 text-primary font-medium border border-primary/25 shadow-sm" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
          >
            <Database size={14} />
            Explorer
          </button>
          <button
            aria-current={view === "operations" ? "page" : undefined}
            onClick={() => setView("operations")}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-3.5 py-1.5 text-xs transition md:flex-none ${view === "operations" ? "bg-primary/10 text-primary font-medium border border-primary/25 shadow-sm" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
          >
            <Sliders size={14} />
            Operations
          </button>
        </nav>
        <div className="flex items-center gap-1 md:ml-auto">
          {onOpenTokens && (
            <Button
              variant="ghost"
              size="icon"
              onClick={onOpenTokens}
              aria-label="API Access Tokens"
              title="API Access Tokens"
              className="text-muted-foreground hover:text-amber-500"
            >
              <Key size={17} />
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={onRefresh}
            disabled={busy}
            aria-label="Refresh data"
            title="Refresh data"
            className="text-muted-foreground hover:text-foreground"
          >
            <RefreshCw size={17} className={busy ? "animate-spin" : ""} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={theme.toggle}
            aria-label={theme.dark ? "Use light theme" : "Use dark theme"}
            title="Toggle theme"
            className="text-muted-foreground hover:text-foreground"
          >
            {theme.dark ? <Sun size={17} /> : <Moon size={17} />}
          </Button>
          <div className="mx-2 hidden h-5 w-px bg-border sm:block" />
          <div className="hidden items-center gap-2 sm:flex">
            <div
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-beast-lime text-[11px] font-bold text-black"
              aria-hidden="true"
            >
              {(user?.username || "??").slice(0, 2).toUpperCase()}
            </div>
            <span
              className="max-w-24 truncate text-xs font-mono text-muted-foreground"
              title={user?.username || "Session unavailable"}
            >
              {user?.username || "Account"}
            </span>
            <RoleBadge role={user?.role} />
          </div>
          <form action="/logout" method="POST">
            <Button
              variant="ghost"
              size="icon"
              type="submit"
              aria-label="Sign out"
              title="Sign out"
              className="text-muted-foreground hover:text-foreground"
            >
              <LogOut size={17} />
            </Button>
          </form>
        </div>
      </div>
    </header>
  );
}
