import { describe, expect, test } from "bun:test";
import { groupSetupKey, looksLikeCode, twoFactorError } from "./twofactor";

describe("two-factor helpers", () => {
  test("groups the setup key in fours", () => {
    expect(groupSetupKey("ABCDEFGHIJKLMNOP")).toBe("ABCD EFGH IJKL MNOP");
    expect(groupSetupKey("ABCDEFG")).toBe("ABCD EFG");
  });
  test("recognises code shapes", () => {
    expect(looksLikeCode("123 456")).toBe(true);
    expect(looksLikeCode("ABCDE-FGHJK")).toBe(true);
    expect(looksLikeCode("12345")).toBe(false);
    expect(looksLikeCode("")).toBe(false);
  });
  test("maps statuses to messages", () => {
    expect(twoFactorError(401, "Invalid code")).toContain("not accepted");
    expect(twoFactorError(429, "")).toContain("Too many");
    expect(twoFactorError(409, "Two-factor is required for administrators\n")).toBe("Two-factor is required for administrators");
  });
});
