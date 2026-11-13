import type { SessionUser, TelemetryStats } from "../../types";

/** Props every top-level Studio page receives from app/page.tsx. */
export interface ViewProps {
  stats: TelemetryStats | null;
  user: SessionUser | null;
  onNotice: (message: string, error?: boolean) => void;
  onRefresh: () => void;
}
