"use client";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

export type Field = {
  id: string;
  name: string;
  type: "string" | "number" | "boolean" | "json";
  value: string;
};
export function buildFields(fields: Field[]): string {
  const data: Record<string, unknown> = Object.create(null);
  for (const field of fields) {
    if (!field.name.trim()) throw new Error("Each property needs a name.");
    if (Object.hasOwn(data, field.name))
      throw new Error(`Duplicate property: ${field.name}`);
    if (field.type === "number") {
      if (!field.value.trim() || !Number.isFinite(Number(field.value)))
        throw new Error(`Enter a valid number for ${field.name}.`);
      data[field.name] = Number(field.value);
    } else if (field.type === "boolean")
      data[field.name] = field.value === "true";
    else if (field.type === "json") {
      try {
        data[field.name] = JSON.parse(field.value);
      } catch {
        throw new Error(`Invalid JSON in ${field.name}.`);
      }
    } else data[field.name] = field.value;
  }
  return JSON.stringify(data, null, 2);
}

export function RecordFields({
  fields,
  setFields,
}: {
  fields: Field[];
  setFields: React.Dispatch<React.SetStateAction<Field[]>>;
}) {
  const update = (id: string, patch: Partial<Field>) =>
    setFields((current) =>
      current.map((f) => (f.id === id ? { ...f, ...patch } : f)),
    );
  return (
    <div className="mt-3 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Properties</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() =>
            setFields((current) => [
              ...current,
              { id: crypto.randomUUID(), name: "", type: "string", value: "" },
            ])
          }
        >
          <Plus size={14} />
          Add property
        </Button>
      </div>
      <div className="max-h-64 space-y-2 overflow-y-auto">
        {fields.map((f) => (
          <div
            key={f.id}
            className="grid grid-cols-[1fr_auto] gap-2 rounded-md border p-2 sm:grid-cols-[1fr_100px_1fr_auto]"
          >
            <Input
              aria-label="Property name"
              placeholder="Name"
              value={f.name}
              onChange={(e) => update(f.id, { name: e.target.value })}
            />
            <select
              aria-label="Property type"
              className="h-9 rounded-md border bg-card px-2 text-sm"
              value={f.type}
              onChange={(e) =>
                update(f.id, { type: e.target.value as Field["type"] })
              }
            >
              <option value="string">String</option>
              <option value="number">Number</option>
              <option value="boolean">Boolean</option>
              <option value="json">JSON</option>
            </select>
            {f.type === "boolean" ? (
              <select
                aria-label="Property value"
                className="h-9 rounded-md border bg-card px-2 text-sm"
                value={f.value || "false"}
                onChange={(e) => update(f.id, { value: e.target.value })}
              >
                <option value="false">false</option>
                <option value="true">true</option>
              </select>
            ) : (
              <Input
                aria-label="Property value"
                placeholder="Value"
                value={f.value}
                onChange={(e) => update(f.id, { value: e.target.value })}
              />
            )}
            <Button
              type="button"
              size="icon"
              variant="ghost"
              aria-label="Remove property"
              onClick={() =>
                setFields((current) => current.filter((x) => x.id !== f.id))
              }
            >
              <Trash2 size={15} />
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}
