import { describe, expect, test } from "bun:test";
import { beastctlCommand, buildFilterQuery, httpRequest, pythonSnippet } from "./query-builder";

const age = { name: "by-age", type: "int" as const };
const city = { name: "by-city", type: "string" as const };
const spec = (index: typeof age | typeof city, op: "eq" | "gte" | "lte" | "between", value: string, value2 = "", limit = 50) => ({ index, op, value, value2, limit });

describe("filter query builder", () => {
  test("equality on an int index", () => {
    const q = buildFilterQuery(spec(age, "eq", " 42 "));
    expect(q.params).toEqual({ index: "by-age", limit: "50", eq: "42" });
    expect(beastctlCommand(q)).toBe("beastctl scan --addr 127.0.0.1:50051 --index by-age --eq 42 --limit 50");
    expect(httpRequest(q)).toBe("GET /api/records?index=by-age&limit=50&eq=42");
  });

  test("between sets both inclusive bounds and rejects an inverted range", () => {
    const q = buildFilterQuery(spec(age, "between", "21", "30"));
    expect(q.params.gte).toBe("21");
    expect(q.params.lte).toBe("30");
    expect(() => buildFilterQuery(spec(age, "between", "31", "30"))).toThrow(/above/);
  });

  test("int validation: whole numbers within 64 bits only", () => {
    expect(() => buildFilterQuery(spec(age, "eq", "1.5"))).toThrow(/whole number/);
    expect(() => buildFilterQuery(spec(age, "eq", "abc"))).toThrow();
    expect(() => buildFilterQuery(spec(age, "eq", "9223372036854775808"))).toThrow(/64 bits/);
    expect(buildFilterQuery(spec(age, "eq", "-9223372036854775808")).params.eq).toBe("-9223372036854775808");
    expect(() => buildFilterQuery(spec(age, "eq", ""))).toThrow(/Enter/);
  });

  test("string values keep spaces and quote safely in a shell and in Python", () => {
    const q = buildFilterQuery(spec(city, "eq", "New York's"));
    expect(beastctlCommand(q)).toContain(`--eq 'New York'\\''s'`);
    expect(pythonSnippet(q, "string")).toContain(`eq=beastdb_pb2.IndexValue(string_value="New York's")`);
    expect(httpRequest(q)).toContain("eq=New%20York's");
    expect(() => buildFilterQuery(spec(city, "eq", "x".repeat(257)))).toThrow(/256/);
  });

  test("limit is clamped to the server cap and must be positive", () => {
    expect(buildFilterQuery(spec(age, "gte", "1", "", 9999)).params.limit).toBe("500");
    expect(() => buildFilterQuery(spec(age, "gte", "1", "", 0))).toThrow(/Limit/);
  });

  test("python snippet uses int_value for int indexes", () => {
    const q = buildFilterQuery(spec(age, "gte", "5"));
    expect(pythonSnippet(q, "int")).toContain("gte=beastdb_pb2.IndexValue(int_value=5)");
  });
});
