export interface EncryptionState {
  encryption_at_rest?: boolean;
  encryption_cipher?: string;
  encryption_key_id?: number;
  /** Kind of master key source ("file", "env", "exec"); never its name or value. */
  encryption_key_source?: string;
  audit_log_encrypted?: boolean;
}

/**
 * Label for the Settings row. It reflects only what the running engine
 * reports; an absent or false field (an older server) reads as Off, never On.
 */
export function encryptionLabel(s: EncryptionState): string {
  if (s.encryption_at_rest !== true) return "Off";
  const parts = [s.encryption_cipher, s.encryption_key_id ? `key ${s.encryption_key_id}` : ""].filter(Boolean);
  return parts.length ? `On (${parts.join(", ")})` : "On";
}

const KEY_SOURCES: Record<string, string> = {
  file: "Key file",
  env: "Environment variable",
  exec: "Command (KMS hook)",
};

/** Master key source row; empty when the engine is not encrypted or did not report one. */
export function keySourceLabel(s: EncryptionState): string {
  if (s.encryption_at_rest !== true || !s.encryption_key_source) return "";
  return KEY_SOURCES[s.encryption_key_source] ?? "Unknown";
}

/** Audit log row: only a server that reports the field is believed; true is the only "Encrypted". */
export function auditLogLabel(s: EncryptionState): string {
  if (s.audit_log_encrypted === undefined) return "";
  return s.audit_log_encrypted === true ? "Encrypted" : "Plaintext";
}
