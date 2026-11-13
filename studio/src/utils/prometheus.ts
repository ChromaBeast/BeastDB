/** Minimal Prometheus text exposition (0.0.4) parser, plus helpers for counters, gauges and histograms. */

export type MetricType = "counter" | "gauge" | "histogram" | "summary" | "untyped";
export type Labels = Record<string, string>;

export interface Sample {
  /** Full sample name, including _bucket/_sum/_count suffixes. */
  name: string;
  labels: Labels;
  value: number;
}

export interface Family {
  name: string;
  type: MetricType;
  help: string;
  samples: Sample[];
}

export type Snapshot = Map<string, Family>;

function parseValue(s: string): number {
  if (s === "+Inf" || s === "Inf") return Infinity;
  if (s === "-Inf") return -Infinity;
  return Number(s); // "NaN" parses to NaN
}

/** Parses a label block body (between braces) into a map. Returns null on malformed input. */
function parseLabels(src: string): Labels | null {
  const out: Labels = {};
  let i = 0;
  while (i < src.length) {
    while (src[i] === " " || src[i] === ",") i++;
    if (i >= src.length) break;
    const eq = src.indexOf("=", i);
    if (eq < 0 || src[eq + 1] !== '"') return null;
    const key = src.slice(i, eq).trim();
    let j = eq + 2;
    let val = "";
    for (; j < src.length && src[j] !== '"'; j++) {
      if (src[j] === "\\" && j + 1 < src.length) {
        j++;
        val += src[j] === "n" ? "\n" : src[j];
      } else val += src[j];
    }
    if (j >= src.length) return null;
    out[key] = val;
    i = j + 1;
  }
  return out;
}

function familyOf(name: string, types: Map<string, MetricType>): string {
  if (types.has(name)) return name;
  for (const suffix of ["_bucket", "_sum", "_count"]) {
    if (name.endsWith(suffix)) {
      const base = name.slice(0, -suffix.length);
      const t = types.get(base);
      if (t === "histogram" || t === "summary") return base;
    }
  }
  return name;
}

/** Parses Prometheus text into families keyed by base name. Malformed lines are skipped. */
export function parsePrometheus(text: string): Snapshot {
  const types = new Map<string, MetricType>();
  const helps = new Map<string, string>();
  const out: Snapshot = new Map();
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith("#")) {
      const m = /^#\s+(HELP|TYPE)\s+(\S+)\s*(.*)$/.exec(line);
      if (!m) continue;
      if (m[1] === "TYPE") types.set(m[2], m[3].trim() as MetricType);
      else helps.set(m[2], m[3]);
      continue;
    }
    const m = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.*)\})?\s+(\S+)(?:\s+\S+)?$/.exec(line);
    if (!m) continue;
    const labels = m[2] === undefined ? {} : parseLabels(m[2]);
    if (!labels) continue;
    const value = parseValue(m[3]);
    const base = familyOf(m[1], types);
    let fam = out.get(base);
    if (!fam) {
      fam = { name: base, type: types.get(base) ?? "untyped", help: helps.get(base) ?? "", samples: [] };
      out.set(base, fam);
    }
    fam.samples.push({ name: m[1], labels, value });
  }
  return out;
}

function matches(s: Sample, want?: Labels): boolean {
  if (!want) return true;
  for (const k of Object.keys(want)) if (s.labels[k] !== want[k]) return false;
  return true;
}

/** Sums sample values of a counter/gauge family, optionally filtered by labels. Undefined when absent. */
export function sumSamples(snap: Snapshot, name: string, want?: Labels, filter?: (s: Sample) => boolean): number | undefined {
  const fam = snap.get(name);
  if (!fam) return undefined;
  let total = 0;
  let any = false;
  for (const s of fam.samples) {
    if (s.name !== name || !matches(s, want) || (filter && !filter(s))) continue;
    total += s.value;
    any = true;
  }
  return any ? total : undefined;
}

/** Value of a (typically unlabelled) gauge family; undefined when absent. */
export function gauge(snap: Snapshot, name: string): number | undefined {
  return sumSamples(snap, name);
}

export interface Histogram {
  /** Cumulative buckets sorted by upper bound, merged across label sets. */
  buckets: { le: number; count: number }[];
  sum: number;
  count: number;
}

/** Merges all label sets of a histogram family into one histogram. Undefined when absent or empty. */
export function histogram(snap: Snapshot, name: string): Histogram | undefined {
  const fam = snap.get(name);
  if (!fam || fam.type !== "histogram") return undefined;
  const byLe = new Map<number, number>();
  let sum = 0;
  let count = 0;
  for (const s of fam.samples) {
    if (s.name === `${name}_bucket` && s.labels.le !== undefined) {
      const le = parseValue(s.labels.le);
      byLe.set(le, (byLe.get(le) ?? 0) + s.value);
    } else if (s.name === `${name}_sum`) sum += s.value;
    else if (s.name === `${name}_count`) count += s.value;
  }
  if (byLe.size === 0) return undefined;
  const buckets = [...byLe.entries()].sort((a, b) => a[0] - b[0]).map(([le, c]) => ({ le, count: c }));
  return { buckets, sum, count };
}

/** Per-bucket difference between two cumulative histograms (current minus previous); counter resets yield undefined. */
export function histogramDelta(cur: Histogram, prev: Histogram): Histogram | undefined {
  if (cur.count < prev.count || cur.buckets.length !== prev.buckets.length) return undefined;
  const buckets = cur.buckets.map((b, i) => ({ le: b.le, count: b.count - prev.buckets[i].count }));
  if (buckets.some((b) => b.count < 0)) return undefined;
  return { buckets, sum: cur.sum - prev.sum, count: cur.count - prev.count };
}

/** Estimates a quantile (0..1) by linear interpolation within the bucket, like PromQL histogram_quantile. */
export function quantile(h: Histogram, q: number): number | undefined {
  const total = h.buckets.length ? h.buckets[h.buckets.length - 1].count : 0;
  if (total <= 0) return undefined;
  const rank = q * total;
  let prevLe = 0;
  let prevCount = 0;
  for (const b of h.buckets) {
    if (b.count >= rank) {
      if (!Number.isFinite(b.le)) return prevLe; // in the +Inf bucket: best known lower bound
      const span = b.count - prevCount;
      if (span <= 0) return b.le;
      return prevLe + ((b.le - prevLe) * (rank - prevCount)) / span;
    }
    prevLe = b.le;
    prevCount = b.count;
  }
  return prevLe;
}

/** Per-second rate between two counter readings; undefined on reset or non-positive elapsed time. */
export function rate(cur: number | undefined, prev: number | undefined, dtSeconds: number): number | undefined {
  if (cur === undefined || prev === undefined || dtSeconds <= 0 || cur < prev) return undefined;
  return (cur - prev) / dtSeconds;
}
