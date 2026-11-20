/** Helpers for the Backups page: restore command generation and readiness checks. */

export interface BackupFacts {
  id: string;
  file_size: number;
  encrypted?: boolean;
  verified: { ok: boolean; at: string; error?: string } | null;
}

/** Quote one argument for a POSIX shell. Plain paths stay unquoted so the command reads naturally. */
export function shellQuote(s: string): string {
  return /^[A-Za-z0-9_@%+=:,./~-]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`;
}

/**
 * The command that restores one server-made bundle into a NEW data directory.
 * It mirrors `beastctl restore --new-dir`, which refuses an existing non-empty
 * target, so the command cannot overwrite a live database.
 */
export function restoreNewCommand(opts: { dir: string; id: string; newDir: string; keyFile?: string; encrypted?: boolean }): string {
  const parts = ["beastctl restore", "--backup", shellQuote(`${opts.dir}/${opts.id}`), "--new-dir", shellQuote(opts.newDir || "./restored")];
  if (opts.encrypted) parts.push("--key-file", shellQuote(opts.keyFile?.trim() || "/path/to/master.key"));
  return parts.join(" ");
}

export type Readiness = { label: string; state: "ok" | "warn" | "bad"; text: string };

/** What an operator should know before running the restore command for this backup. */
export function restoreReadiness(b: BackupFacts, formatBytes: (n: number) => string): Readiness[] {
  const out: Readiness[] = [];
  if (!b.verified) out.push({ label: "Verification", state: "warn", text: "Not verified since the server started. restore verifies first, or press Verify." });
  else if (b.verified.ok) out.push({ label: "Verification", state: "ok", text: "Passed (checksum, size and pages)." });
  else out.push({ label: "Verification", state: "bad", text: `Failed: ${b.verified.error ?? "unknown error"}. Do not restore from this bundle.` });
  out.push(b.encrypted
    ? { label: "Encryption", state: "warn", text: "Encrypted. The master key file is needed; it is never stored in the bundle." }
    : { label: "Encryption", state: "ok", text: "Not encrypted. No key file needed." });
  out.push({ label: "Disk needed", state: "ok", text: `About ${formatBytes(b.file_size)} free beside the new directory (it stages first).` });
  return out;
}
