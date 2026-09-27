import type { PayloadFormat, UniversalRecord } from "../types";

const FORMAT_LABELS: Record<PayloadFormat, string> = {
  json_object: "Document",
  json_array: "List",
  token: "Delimited text",
  string: "Text",
  binary: "Binary data",
  empty: "Empty record",
};

export function formatLabel(format: PayloadFormat): string {
  return FORMAT_LABELS[format];
}

export function fieldLabel(name: string): string {
  return name
    .replace(/([a-z\d])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/^./, (first) => first.toUpperCase())
    .replace(/\bId\b/g, "ID");
}

export function recordPreview(record: UniversalRecord): string {
  if (record.secondaryLabel) return record.secondaryLabel;
  if (record.fields) {
    const summary = record.attributes
      .filter((field) => !["name", "title", "displayName", "username", "id"].includes(field.key))
      .slice(0, 2)
      .map((field) => `${fieldLabel(field.key)}: ${field.value}`)
      .join(" · ");
    if (summary) return summary;
  }
  if (record.format === "json_array") return `${record.arrayItems?.length ?? 0} items`;
  if (record.format === "empty") return "No value";
  return formatLabel(record.format);
}
