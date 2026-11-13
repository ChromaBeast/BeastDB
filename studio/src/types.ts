export type PayloadFormat =
  "json_object" | "json_array" | "token" | "string" | "binary" | "empty";

export interface ParsedField {
  key: string;
  value: any;
  type:
    | "string"
    | "number"
    | "boolean"
    | "array"
    | "object"
    | "date"
    | "url"
    | "image"
    | "null";
}

export interface UniversalRecord {
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

export interface StorageMetrics {
  totalPages: number;
  diskSizeBytes: number;
  poolSize: number;
  cachedPages: number;
  pinnedPages: number;
  dirtyPages: number;
  walSizeBytes: number;
  currentLSN: number;
  activeDataPage: number;
  lastCheckpointLSN?: number;
  /** Unix seconds; 0 until a checkpoint completes in this process. */
  lastCheckpointUnix?: number;
  readOnly?: boolean;
}

export interface ReplicaStatus {
  id: string;
  address?: string;
  connected: boolean;
  ackedLSN: number;
  lagLSN: number;
}

/** This node's epoch and promotion state (absent when the node does not track epochs). */
export interface ClusterStatus {
  nodeID: string;
  role: "leader" | "follower" | "fenced";
  epoch: number;
  fencedFor?: string;
  appliedLSN: number;
  leaderLSN: number;
  /** Milliseconds since a current leader last answered; -1 if never. */
  leaderContactMS: number;
  lagLSN: number;
  promotable: boolean;
  promoteBlockedReason?: string;
}

export interface ReplicationStatus {
  role: string;
  lsn: number;
  /** True only on a leader; followers cannot see other replicas. */
  tracked: boolean;
  replicas: ReplicaStatus[];
  cluster?: ClusterStatus;
}

export interface TelemetryStats {
  lsn: number;
  role: string;
  mode: string;
  version: string;
  partitionCounts?: Record<string, number>;
  storage?: StorageMetrics;
}

export interface SessionUser {
  username: string;
  role: "admin" | "viewer";
}

export type ViewMode = "split" | "table" | "grid";

export interface DomainDistribution {
  prefix: number;
  label: string;
  count: number;
  color: string;
}

export interface PartitionConfig {
  prefix: number;
  label: string;
  color: string;
  description?: string;
  icon?: string;
}

export interface APITokenItem {
  id: string;
  name: string;
  token_hash: string;
  masked_token: string;
  role: string;
  created_at: string;
  created_by: string;
}

export interface SearchResponse {
  records: UniversalRecord[];
  count: number;
  scannedCount: number;
  nextKey?: string;
  hasMore: boolean;
  prefix?: number;
}


