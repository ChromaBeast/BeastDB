import { UniversalRecord, PayloadFormat, ParsedField } from "../types";
import { decodeKey } from "./key-decoder";

export function inferFieldType(val: any): ParsedField["type"] {
  if (val === null || val === undefined) return "null";
  if (Array.isArray(val)) return "array";
  const t = typeof val;
  if (t === "boolean") return "boolean";
  if (t === "number") return "number";
  if (t === "object") return "object";
  if (t === "string") {
    if (val.match(/^https?:\/\/.+\.(jpg|jpeg|png|webp|gif|svg)(\?.*)?$/i) ||
        val.includes("steamstatic.com") || val.includes("tmdb.org")) {
      return "image";
    }
    if (val.match(/^https?:\/\//i)) return "url";
    if (val.match(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/)) return "date";
    return "string";
  }
  return "string";
}

function findImage(obj: any): string | undefined {
  if (!obj || typeof obj !== "object") return undefined;
  const candidates = [
    obj.coverUrl, obj.posterUrl, obj.avatarUrl,
    obj.cover_url, obj.poster_url, obj.avatar_url, obj.avatar,
    obj.thumbnail, obj.image, obj.icon, obj.photo,
  ];
  for (const c of candidates) {
    if (typeof c === "string" && c.startsWith("http")) return c;
  }
  for (const v of Object.values(obj)) {
    if (typeof v === "string" && v.match(/^https?:\/\/.+\.(jpg|jpeg|png|webp|gif|svg)(\?.*)?$/i)) {
      return v;
    }
  }
  return undefined;
}

const REDACTED_KEYS = new Set([
  "passwordHash", "password_hash", "password", "salt", "password_salt",
  "token", "secret", "apiKey", "api_key", "apiSecret", "api_secret",
  "privateKey", "private_key", "sessionToken", "session_secret",
  "api_token", "refresh_token",
]);

function displayText(data: Record<string, any>, names: string[]): string | undefined {
  for (const [key, value] of Object.entries(data)) {
    if (!names.includes(key.replace(/[_-]/g, "").toLowerCase())) continue;
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return undefined;
}

function displayName(data: Record<string, any>, depth = 0): string | undefined {
  const direct = displayText(data, ["title", "displayname", "name", "label"]);
  if (direct || depth >= 2) return direct;
  for (const value of Object.values(data)) {
    if (value && typeof value === "object" && !Array.isArray(value)) {
      const nested = displayName(value, depth + 1);
      if (nested) return nested;
    }
  }
  return undefined;
}

export function parseUniversalRecord(rawItem: { keyText: string; value: string }): UniversalRecord {
  const decoded = decodeKey(rawItem.keyText);
  const rawStr = rawItem.value ?? "";
  const byteSize = new Blob([rawStr]).size;
  const trimmed = rawStr.trim();

  let format: PayloadFormat = "string";
  let fields: Record<string, any> | undefined = undefined;
  let arrayItems: any[] | undefined = undefined;
  let primaryLabel = "";
  let secondaryLabel = "";
  let coverUrl: string | undefined = undefined;
  let status: string | undefined = undefined;
  let rating: number | string | undefined = undefined;
  const attributes: { key: string; value: string; type: string }[] = [];

  if (!trimmed) {
    format = "empty";
    primaryLabel = "Empty record";
  } else if (trimmed.startsWith("{") && trimmed.endsWith("}")) {
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
        format = "json_object";
        // Filter sensitive fields for display; parsed is still used for label/cover extraction
        fields = Object.fromEntries(
          Object.entries(parsed).filter(([k]) => !REDACTED_KEYS.has(k))
        );
        coverUrl = findImage(parsed);

        const name = displayName(parsed) ?? displayText(parsed, ["username", "email"]);
        const note = displayText(parsed, ["description", "summary", "subtitle", "notes"]);
        primaryLabel = name ?? note ?? (parsed.id != null ? `Record ${parsed.id}` : `Record ${decoded.raw.slice(-6)}`);
        secondaryLabel = [parsed.status, name ? note : undefined].filter(Boolean).join(" · ");
        status = parsed.status ?? parsed.role ?? parsed.type;
        rating = parsed.rating ?? parsed.score ?? parsed.userRating;

        // Collect top summary attributes
        for (const [k, v] of Object.entries(parsed)) {
          if (REDACTED_KEYS.has(k)) continue;
          if (["coverUrl", "posterUrl", "avatarUrl", "description"].includes(k)) continue;
          if (typeof v === "object" && v !== null && !Array.isArray(v)) {
            for (const [subK, subV] of Object.entries(v)) {
              if (attributes.length < 5 && typeof subV !== "object") {
                attributes.push({ key: `${k}.${subK}`, value: String(subV), type: typeof subV });
              }
            }
          } else if (attributes.length < 5) {
            attributes.push({ key: k, value: Array.isArray(v) ? `[${v.length}]` : String(v), type: typeof v });
          }
        }
      }
    } catch {
      format = "string";
    }
  } else if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
    try {
      const parsed = JSON.parse(trimmed);
      if (Array.isArray(parsed)) {
        format = "json_array";
        arrayItems = parsed;
        primaryLabel = `Array [${parsed.length} items]`;
        attributes.push({ key: "length", value: String(parsed.length), type: "number" });
      }
    } catch {
      format = "string";
    }
  } else if (trimmed.includes("|") && trimmed.length < 250) {
    format = "token";
    const parts = trimmed.split("|");
    primaryLabel = parts[0] || "Token Payload";
    secondaryLabel = parts.slice(1).join(" • ");
    parts.forEach((p, idx) => attributes.push({ key: `part_${idx + 1}`, value: p, type: "string" }));
  }

  if (format === "string") {
    primaryLabel = trimmed.length > 50 ? trimmed.slice(0, 47) + "..." : trimmed;
    attributes.push({ key: "length", value: `${trimmed.length} chars`, type: "number" });
  }

  return {
    keyStr: decoded.raw,
    keyHex: decoded.hex,
    prefix: decoded.prefix,
    prefixLabel: decoded.prefixLabel,
    userHash: decoded.userHash,
    itemHash: decoded.itemHash,
    format,
    byteSize,
    primaryLabel: primaryLabel || `Key ${decoded.prefixHex}`,
    secondaryLabel,
    coverUrl,
    status,
    rating,
    attributes,
    fields,
    arrayItems,
    raw: rawStr,
  };
}
