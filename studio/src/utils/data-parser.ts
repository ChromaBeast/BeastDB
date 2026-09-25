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
  const candidates = [obj.coverUrl, obj.posterUrl, obj.avatarUrl, obj.thumbnail, obj.image, obj.icon,
                      obj.game?.coverUrl, obj.movie?.posterUrl, obj.tv?.posterUrl, obj.book?.coverUrl];
  for (const c of candidates) {
    if (typeof c === "string" && c.startsWith("http")) return c;
  }
  for (const k of Object.keys(obj)) {
    const v = obj[k];
    if (typeof v === "string" && (v.includes("steamstatic.com") || v.includes("tmdb.org") || v.match(/\.(jpg|jpeg|png|webp)$/i))) {
      return v;
    }
  }
  return undefined;
}

export function parseUniversalRecord(rawItem: { key: number; value: string }): UniversalRecord {
  const decoded = decodeKey(rawItem.key);
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
    primaryLabel = `[Empty Record ${decoded.prefixHex}]`;
  } else if (trimmed.startsWith("{") && trimmed.endsWith("}")) {
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)) {
        format = "json_object";
        fields = parsed;
        coverUrl = findImage(parsed);
        const inner = parsed.game || parsed.movie || parsed.tv || parsed.book || parsed.user || {};

        primaryLabel = String(parsed.title || inner.title || parsed.name || inner.name ||
                              parsed.username || parsed.email || parsed.id || `Entity ${decoded.prefixHex}`);
        secondaryLabel = String(inner.developer || inner.releaseYear || parsed.description || parsed.notes || "");
        status = parsed.status || inner.status || parsed.role;
        rating = parsed.rating ?? inner.rating ?? parsed.userRating;

        // Collect top summary attributes
        for (const [k, v] of Object.entries(parsed)) {
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
    key: rawItem.key,
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
