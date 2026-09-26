"use client";
import {
  Activity,
  Database,
  LogOut,
  Moon,
  RefreshCw,
  Sun,
} from "lucide-react";
import { Button } from "./ui/button";
import { BeastDBLogo } from "./BeastDBLogo";
import { SessionUser } from "../types";
import { useTheme } from "../hooks/useTheme";
import { RoleBadge } from "./RoleBadge";

interface Props {
  view: "records" | "overview";
  setView: (view: "records" | "overview") => void;
  user: SessionUser | null;
  busy: boolean;
  onRefresh: () => void;
}
export function StudioHeader({ view, setView, user, busy, onRefresh }: Props) {
  const theme = useTheme();
  return (
    <header className="sticky top-0 z-40 border-b bg-card/95 backdrop-blur">
      <div className="mx-auto flex max-w-[1600px] flex-wrap items-center gap-3 px-4 py-3 md:px-8">
        <div className="mr-auto flex items-center gap-3">
          <BeastDBLogo />
        </div>
        <nav
          aria-label="Main navigation"
          className="order-3 flex w-full gap-1 rounded-lg bg-muted p-1 md:order-none md:w-auto"
        >
          <button
            aria-current={view === "records" ? "page" : undefined}
            onClick={() => setView("records")}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-1.5 text-sm md:flex-none ${view === "records" ? "bg-card font-medium shadow-sm" : "text-muted-foreground hover:text-foreground"}`}
          >
            <Database size={15} />
            Records
          </button>
          <button
            aria-current={view === "overview" ? "page" : undefined}
            onClick={() => setView("overview")}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-1.5 text-sm md:flex-none ${view === "overview" ? "bg-card font-medium shadow-sm" : "text-muted-foreground hover:text-foreground"}`}
          >
            <Activity size={15} />
            Overview
          </button>
        </nav>
        <div className="flex items-center gap-1 md:ml-auto">
          <Button
            variant="ghost"
            size="icon"
            onClick={onRefresh}
            disabled={busy}
            aria-label="Refresh data"
            title="Refresh data"
          >
            <RefreshCw size={17} className={busy ? "animate-spin" : ""} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={theme.toggle}
            aria-label={theme.dark ? "Use light theme" : "Use dark theme"}
            title="Toggle theme"
          >
            {theme.dark ? <Sun size={17} /> : <Moon size={17} />}
          </Button>
          <div className="mx-2 hidden h-5 w-px bg-border sm:block" />
          <div className="hidden items-center gap-2 sm:flex">
            <div
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary text-[11px] font-semibold text-primary-foreground"
              aria-hidden="true"
            >
              {(user?.username || "??").slice(0, 2).toUpperCase()}
            </div>
            <span
              className="max-w-24 truncate text-xs text-muted-foreground"
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
            >
              <LogOut size={17} />
            </Button>
          </form>
        </div>
      </div>
    </header>
  );
}
