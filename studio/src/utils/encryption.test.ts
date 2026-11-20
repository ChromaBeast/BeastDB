import { describe, expect, test } from "bun:test";
import { encryptionLabel } from "./encryption";

describe("encryptionLabel", () => {
  test("is Off when the server does not report encryption", () => {
    expect(encryptionLabel({})).toBe("Off");
    expect(encryptionLabel({ encryption_cipher: "aes-256-gcm", encryption_key_id: 1 })).toBe("Off");
    expect(encryptionLabel({ encryption_at_rest: false })).toBe("Off");
  });
  test("is On only when the engine says so", () => {
    expect(encryptionLabel({ encryption_at_rest: true, encryption_cipher: "aes-256-gcm", encryption_key_id: 2 })).toBe("On (aes-256-gcm, key 2)");
    expect(encryptionLabel({ encryption_at_rest: true })).toBe("On");
  });
});
