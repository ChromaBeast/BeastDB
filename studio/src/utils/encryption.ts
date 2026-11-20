export interface EncryptionState {
  encryption_at_rest?: boolean;
  encryption_cipher?: string;
  encryption_key_id?: number;
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
