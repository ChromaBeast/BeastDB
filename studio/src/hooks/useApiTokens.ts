"use client";
import { useCallback, useEffect, useState } from "react";
import type { APITokenItem } from "../types";

/** Token as returned by GET /api/tokens (expires_at is omitted for non-expiring tokens). */
/** last_used_at is in-memory on the server: absent until the key is used after a restart. */
export type ApiToken = APITokenItem & { expires_at?: string; last_used_at?: string };

/** Roles accepted by POST /api/tokens (api.ValidTokenRole). */
export const TOKEN_ROLES = ["admin", "writer", "viewer", "replication"] as const;
export type TokenRole = (typeof TOKEN_ROLES)[number];

/** Expiry presets; 0 means the token never expires. Server cap is 10 years. */
export const EXPIRY_OPTIONS: { label: string; seconds: number }[] = [
  { label: "Never", seconds: 0 },
  { label: "24 hours", seconds: 24 * 3600 },
  { label: "7 days", seconds: 7 * 24 * 3600 },
  { label: "30 days", seconds: 30 * 24 * 3600 },
  { label: "90 days", seconds: 90 * 24 * 3600 },
  { label: "1 year", seconds: 365 * 24 * 3600 },
];

export interface CreateTokenInput {
  name: string;
  role: string;
  expiresInSeconds: number;
}

export function isExpired(t: ApiToken, now = Date.now()): boolean {
  return !!t.expires_at && new Date(t.expires_at).getTime() <= now;
}

/** Loads and mutates API tokens via /api/tokens. Pass enabled=false to skip fetching (e.g. closed dialog, non-admin). */
export function useApiTokens(enabled: boolean, onNotice: (msg: string, err?: boolean) => void) {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(false);
  const [forbidden, setForbidden] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch("/api/tokens");
      if (res.status === 401 || res.status === 403) {
        setForbidden(true);
        setTokens([]);
        return;
      }
      if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
      const json = await res.json();
      setForbidden(false);
      setError(null);
      setTokens(json.tokens || []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  /** Creates a token; resolves with the one-time secret, or null on failure (after notifying). */
  const create = useCallback(
    async (input: CreateTokenInput): Promise<string | null> => {
      try {
        const res = await fetch("/api/tokens", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            name: input.name.trim(),
            role: input.role,
            ...(input.expiresInSeconds > 0 ? { expires_in_seconds: input.expiresInSeconds } : {}),
          }),
        });
        if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
        const data = await res.json();
        onNotice("API token generated and saved in partition 0x05.");
        await refresh();
        return data.token as string;
      } catch (e) {
        onNotice((e as Error).message, true);
        return null;
      }
    },
    [onNotice, refresh],
  );

  const revoke = useCallback(
    async (id: string): Promise<boolean> => {
      try {
        const res = await fetch(`/api/tokens?id=${encodeURIComponent(id)}`, { method: "DELETE" });
        if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
        onNotice("Token revoked.");
        await refresh();
        return true;
      } catch (e) {
        onNotice((e as Error).message, true);
        return false;
      }
    },
    [onNotice, refresh],
  );

  return { tokens, loading, forbidden, error, refresh, create, revoke };
}
