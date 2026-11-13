import { expect, test } from "bun:test";
import { gauge, histogram, histogramDelta, parsePrometheus, quantile, rate, sumSamples } from "./prometheus";

const TEXT = `# HELP beastdb_uptime_seconds Seconds since process start.
# TYPE beastdb_uptime_seconds gauge
beastdb_uptime_seconds 12.5
# HELP beastdb_requests_total Requests handled.
# TYPE beastdb_requests_total counter
beastdb_requests_total{protocol="http",method="GET /api",code="200"} 90
beastdb_requests_total{protocol="http",method="GET /api",code="500"} 10
beastdb_requests_total{protocol="grpc",method="Get",code="OK"} 5
# TYPE beastdb_request_duration_seconds histogram
beastdb_request_duration_seconds_bucket{protocol="http",le="0.1"} 50
beastdb_request_duration_seconds_bucket{protocol="http",le="1"} 90
beastdb_request_duration_seconds_bucket{protocol="http",le="+Inf"} 100
beastdb_request_duration_seconds_sum{protocol="http"} 20
beastdb_request_duration_seconds_count{protocol="http"} 100
beastdb_build_info{version="v1 \\"x\\"",role="leader"} 1
bad line here
`;

test("parses families, types, labels and escapes", () => {
  const snap = parsePrometheus(TEXT);
  expect(gauge(snap, "beastdb_uptime_seconds")).toBe(12.5);
  expect(snap.get("beastdb_requests_total")?.type).toBe("counter");
  expect(snap.get("beastdb_requests_total")?.samples.length).toBe(3);
  expect(snap.get("beastdb_build_info")?.samples[0].labels.version).toBe('v1 "x"');
  expect(snap.get("beastdb_request_duration_seconds")?.samples.length).toBe(5);
});

test("sumSamples filters by labels and reports absence", () => {
  const snap = parsePrometheus(TEXT);
  expect(sumSamples(snap, "beastdb_requests_total")).toBe(105);
  expect(sumSamples(snap, "beastdb_requests_total", { protocol: "http" })).toBe(100);
  expect(sumSamples(snap, "beastdb_requests_total", undefined, (s) => s.labels.code.startsWith("5"))).toBe(10);
  expect(sumSamples(snap, "nope")).toBeUndefined();
});

test("histogram quantiles interpolate within buckets", () => {
  const h = histogram(parsePrometheus(TEXT), "beastdb_request_duration_seconds");
  expect(h?.count).toBe(100);
  expect(h?.buckets.length).toBe(3);
  expect(quantile(h!, 0.5)).toBeCloseTo(0.1, 5);
  expect(quantile(h!, 0.7)).toBeCloseTo(0.1 + 0.9 * (20 / 40), 5);
  // p99 lands in +Inf: falls back to the highest finite bound
  expect(quantile(h!, 0.99)).toBe(1);
});

test("histogramDelta and rate handle resets", () => {
  const a = { buckets: [{ le: 1, count: 10 }, { le: Infinity, count: 12 }], sum: 5, count: 12 };
  const b = { buckets: [{ le: 1, count: 4 }, { le: Infinity, count: 5 }], sum: 2, count: 5 };
  expect(histogramDelta(a, b)?.count).toBe(7);
  expect(histogramDelta(b, a)).toBeUndefined();
  expect(rate(110, 100, 5)).toBe(2);
  expect(rate(90, 100, 5)).toBeUndefined();
  expect(rate(undefined, 1, 5)).toBeUndefined();
});
