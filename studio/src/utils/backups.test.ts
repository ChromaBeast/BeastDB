import { describe, expect, test } from "bun:test";
import { restoreNewCommand, restoreReadiness, shellQuote } from "./backups";

const fmt = (n: number) => `${n} B`;

describe("shellQuote", () => {
  test("leaves plain paths alone and quotes the rest", () => {
    expect(shellQuote("/var/lib/beastdb/restored")).toBe("/var/lib/beastdb/restored");
    expect(shellQuote("/my data/x")).toBe("'/my data/x'");
    expect(shellQuote("a'b; rm -rf /")).toBe(`'a'\\''b; rm -rf /'`);
  });
});

describe("restoreNewCommand", () => {
  test("uses --new-dir and never --force or --dest", () => {
    const c = restoreNewCommand({ dir: "/d/backups", id: "bk-20260101-000000", newDir: "./out" });
    expect(c).toBe("beastctl restore --backup /d/backups/bk-20260101-000000 --new-dir ./out");
    expect(c).not.toContain("--force");
    expect(c).not.toContain("--dest");
  });
  test("adds a key file for an encrypted bundle, with a placeholder by default", () => {
    expect(restoreNewCommand({ dir: "b", id: "x", newDir: "n", encrypted: true })).toContain("--key-file /path/to/master.key");
    expect(restoreNewCommand({ dir: "b", id: "x", newDir: "n", encrypted: true, keyFile: " /k y " })).toContain("--key-file '/k y'");
  });
  test("quotes a hostile directory", () => {
    expect(restoreNewCommand({ dir: "b", id: "x", newDir: "$(reboot)" })).toContain("'$(reboot)'");
  });
});

describe("restoreReadiness", () => {
  test("flags failed verification and encryption", () => {
    const r = restoreReadiness({ id: "x", file_size: 10, encrypted: true, verified: { ok: false, at: "", error: "checksum mismatch" } }, fmt);
    expect(r[0].state).toBe("bad");
    expect(r[0].text).toContain("checksum mismatch");
    expect(r[1].state).toBe("warn");
  });
  test("an unverified plain bundle is a warning, not a pass", () => {
    const r = restoreReadiness({ id: "x", file_size: 10, verified: null }, fmt);
    expect(r[0].state).toBe("warn");
    expect(r[1].state).toBe("ok");
  });
});
