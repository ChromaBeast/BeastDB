import { describe, expect, test } from "bun:test";
import { auditLogLabel, encryptionLabel, keySourceLabel } from "./encryption";

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

describe("keySourceLabel and auditLogLabel", () => {
  test("key source is shown only for an encrypted engine that reports one", () => {
    expect(keySourceLabel({})).toBe("");
    expect(keySourceLabel({ encryption_key_source: "env" })).toBe("");
    expect(keySourceLabel({ encryption_at_rest: true })).toBe("");
    expect(keySourceLabel({ encryption_at_rest: true, encryption_key_source: "exec" })).toBe("Command (KMS hook)");
    expect(keySourceLabel({ encryption_at_rest: true, encryption_key_source: "file" })).toBe("Key file");
  });
  test("audit log is Encrypted only when the server says so", () => {
    expect(auditLogLabel({})).toBe("");
    expect(auditLogLabel({ audit_log_encrypted: false })).toBe("Plaintext");
    expect(auditLogLabel({ audit_log_encrypted: true })).toBe("Encrypted");
  });
});
