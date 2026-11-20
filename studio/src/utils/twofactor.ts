/** Group a base32 setup key in blocks of four so it is easy to type: "ABCD EFGH …". */
export function groupSetupKey(secret: string): string {
  return secret.replace(/\s+/g, "").replace(/(.{4})/g, "$1 ").trim();
}

/** A TOTP code is six digits (spaces allowed); a recovery code is ten letters/digits with an optional dash. */
export function looksLikeCode(input: string): boolean {
  const s = input.replace(/[\s-]/g, "");
  return /^\d{6}$/.test(s) || /^[A-Za-z0-9]{10}$/.test(s);
}

/** Turn a failed fetch Response into one readable sentence for the notice bar. */
export function twoFactorError(status: number, body: string): string {
  if (status === 401) return "That code was not accepted. Use the current code from your app, or an unused recovery code.";
  if (status === 429) return "Too many attempts. Wait a moment and try again.";
  return body.trim() || `Request failed (HTTP ${status}).`;
}
