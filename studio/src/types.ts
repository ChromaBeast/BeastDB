export type PayloadFormat =
  | "json_object"
  | "json_array"
  | "token"
  | "string"
  | "binary"
  | "empty";

export interface ParsedField {
  key: string;
  value: any;
  type: "string" | "number" | "boolean" | "array" | "object" | "date" | "url" | "image" | "null";
}

export interface UniversalRecord {
  key: number;
  keyStr: string;
  keyHex: string;
  prefix: number;
  prefixLabel: string;
  userHash?: number;
  itemHash?: number;
  format: PayloadFormat;
  byteSize: number;
  primaryLabel: string;
  secondaryLabel?: string;
  coverUrl?: string;
  status?: string;
  rating?: number | string;
  attributes: { key: string; value: string; type: string }[];
  fields?: Record<string, any>;
  arrayItems?: any[];
  raw: string;
}

export interface TelemetryStats {
  lsn: number;
  role: string;
  mode: string;
  status: string;
  version: string;
}

export type ViewMode = "grid" | "table";

export interface DomainDistribution {
  prefix: number;
  label: string;
  count: number;
  color: string;
}
