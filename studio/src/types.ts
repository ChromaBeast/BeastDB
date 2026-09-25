export interface DbRecord {
  key: number;
  value: string;
  parsed?: {
    title?: string;
    name?: string;
    type?: string;
    status?: string;
    rating?: number | string;
    description?: string;
    created_at?: string;
    [k: string]: any;
  };
}

export interface TelemetryStats {
  lsn: number;
  role: string;
  mode: string;
  status: string;
  version: string;
}

export type CategoryFilter = "all" | "game" | "movie" | "tv" | "book" | "system";
