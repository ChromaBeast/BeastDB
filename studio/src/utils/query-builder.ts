import type { IndexEntry } from "../types";

/** Predicates the server supports on an index: equality or an inclusive range. */
export type FilterOp = "eq" | "gte" | "lte" | "between";

export interface FilterSpec {
  index: Pick<IndexEntry, "name" | "type">;
  op: FilterOp;
  value: string;
  value2: string;
  limit: number;
  /** resume key (decimal) for a following page */
  start?: string;
}

export interface FilterQuery {
  /** Query-string parameters for GET /api/records. */
  params: Record<string, string>;
  /** [flag, value] pairs for `beastctl scan --index`. */
  flags: Array<[string, string]>;
}

const INT_RE = /^-?\d+$/;
const I64_MAX = 9223372036854775807n;
const I64_MIN = -9223372036854775808n;

export const OP_LABEL: Record<FilterOp, string> = {
  eq: "equals",
  gte: "is at least",
  lte: "is at most",
  between: "is between (inclusive)",
};

function checkValue(label: string, type: "int" | "string", v: string): string {
  if (type === "string") {
    if (new TextEncoder().encode(v).length > 256) throw new Error(`${label} is longer than the 256 bytes the index stores.`);
    return v;
  }
  const t = v.trim();
  if (!INT_RE.test(t)) throw new Error(`${label} must be a whole number for an int index.`);
  const n = BigInt(t);
  if (n > I64_MAX || n < I64_MIN) throw new Error(`${label} does not fit in 64 bits.`);
  return n.toString();
}

/** Validates the form and returns the predicate in the server's own terms. Throws a UI-ready Error. */
export function buildFilterQuery(s: FilterSpec): FilterQuery {
  if (!Number.isInteger(s.limit) || s.limit < 1) throw new Error("Limit must be a positive whole number.");
  const params: Record<string, string> = { index: s.index.name, limit: String(Math.min(s.limit, 500)) };
  const flags: Array<[string, string]> = [["index", s.index.name]];
  const add = (name: "eq" | "gte" | "lte", label: string, raw: string) => {
    if (s.index.type === "string" ? raw === "" : raw.trim() === "") throw new Error(`Enter ${label}.`);
    const v = checkValue(label, s.index.type, raw);
    params[name] = v;
    flags.push([name, v]);
  };
  if (s.op === "eq") add("eq", "a value", s.value);
  if (s.op === "gte") add("gte", "a lower bound", s.value);
  if (s.op === "lte") add("lte", "an upper bound", s.value);
  if (s.op === "between") {
    add("gte", "a lower bound", s.value);
    add("lte", "an upper bound", s.value2);
    if (s.index.type === "int" && BigInt(params.gte) > BigInt(params.lte)) throw new Error("The lower bound is above the upper bound.");
    if (s.index.type === "string" && params.gte > params.lte) throw new Error("The lower bound sorts after the upper bound.");
  }
  if (s.start) {
    params.start = s.start;
    flags.push(["start", s.start]);
  }
  if (s.limit > 0) flags.push(["limit", String(Math.min(s.limit, 500))]);
  return { params, flags };
}

function shellQuote(v: string): string {
  return /^[A-Za-z0-9_.:@%+=,-]+$/.test(v) ? v : `'${v.replace(/'/g, `'\\''`)}'`;
}

/** The `beastctl scan` command equal to the query. */
export function beastctlCommand(q: FilterQuery, addr = "127.0.0.1:50051"): string {
  return ["beastctl scan", `--addr ${addr}`, ...q.flags.map(([f, v]) => `--${f} ${shellQuote(v)}`)].join(" ");
}

function pyStr(v: string): string {
  // JSON string syntax is valid Python for the escapes JSON.stringify emits.
  return JSON.stringify(v);
}

/** Python using gRPC stubs generated from api/proto/beastdb.proto (see examples/python/client.py). */
export function pythonSnippet(q: FilterQuery, indexType: "int" | "string", addr = "127.0.0.1:50051"): string {
  const val = (v: string) => (indexType === "int" ? `beastdb_pb2.IndexValue(int_value=${v})` : `beastdb_pb2.IndexValue(string_value=${pyStr(v)})`);
  const parts = [`index=${pyStr(q.params.index)}`];
  for (const k of ["eq", "gte", "lte"] as const) if (q.params[k] !== undefined) parts.push(`${k}=${val(q.params[k])}`);
  return [
    "import grpc, beastdb_pb2, beastdb_pb2_grpc",
    "",
    `stub = beastdb_pb2_grpc.BeastDBServiceStub(grpc.insecure_channel(${pyStr(addr)}))`,
    `meta = (("authorization", "Bearer " + TOKEN),)  # your API token`,
    `req = beastdb_pb2.ScanRequest(limit=${q.params.limit}, filter=beastdb_pb2.IndexFilter(${parts.join(", ")}))`,
    "for item in stub.Scan(req, metadata=meta):",
    "    print(item.key, item.value.decode())",
  ].join("\n");
}

/** The Studio HTTP request (session cookie auth) that returns the same records. */
export function httpRequest(q: FilterQuery): string {
  const qs = Object.entries(q.params).map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join("&");
  return `GET /api/records?${qs}`;
}
