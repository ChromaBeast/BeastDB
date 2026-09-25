"use client";

import React from "react";
import { Plus, Trash2 } from "lucide-react";

export interface CustomFieldRow {
  id: string;
  key: string;
  type: "string" | "number" | "boolean" | "json";
  value: string;
}

interface DynamicFormBuilderProps {
  fields: CustomFieldRow[];
  setFields: React.Dispatch<React.SetStateAction<CustomFieldRow[]>>;
}

export const DynamicFormBuilder: React.FC<DynamicFormBuilderProps> = ({
  fields,
  setFields,
}) => {
  const addField = () => {
    setFields((prev) => [
      ...prev,
      { id: Math.random().toString(36).slice(2), key: "", type: "string", value: "" },
    ]);
  };

  const removeField = (id: string) => {
    setFields((prev) => prev.filter((f) => f.id !== id));
  };

  const updateField = (id: string, updates: Partial<CustomFieldRow>) => {
    setFields((prev) => prev.map((f) => (f.id === id ? { ...f, ...updates } : f)));
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-slate-300">Document Key-Value Properties</span>
        <button
          type="button"
          onClick={addField}
          className="flex items-center gap-1 rounded-lg border border-purple-500/30 bg-purple-500/10 px-2.5 py-1 text-[11px] font-medium text-purple-300 hover:bg-purple-500/20"
        >
          <Plus className="h-3 w-3" />
          <span>Add Property</span>
        </button>
      </div>

      <div className="max-h-60 overflow-y-auto space-y-2 pr-1">
        {fields.length === 0 ? (
          <div className="rounded-xl border border-dashed border-white/10 p-6 text-center text-xs text-slate-500">
            No properties added yet. Click &quot;Add Property&quot; to build a custom schema.
          </div>
        ) : (
          fields.map((f) => (
            <div key={f.id} className="flex items-center gap-2 rounded-xl border border-white/[0.08] bg-black/40 p-2 text-xs">
              <input
                type="text"
                placeholder="Field Name (e.g. title)"
                value={f.key}
                onChange={(e) => updateField(f.id, { key: e.target.value })}
                className="w-1/3 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1 font-mono text-xs text-purple-300 outline-none focus:border-purple-500"
              />
              <select
                value={f.type}
                onChange={(e) => updateField(f.id, { type: e.target.value as any })}
                className="w-24 rounded-lg border border-white/10 bg-black/50 px-2 py-1 text-xs text-white outline-none focus:border-purple-500"
              >
                <option value="string">String</option>
                <option value="number">Number</option>
                <option value="boolean">Boolean</option>
                <option value="json">JSON</option>
              </select>
              {f.type === "boolean" ? (
                <select
                  value={f.value}
                  onChange={(e) => updateField(f.id, { value: e.target.value })}
                  className="flex-1 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1 text-xs text-white outline-none focus:border-purple-500"
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              ) : (
                <input
                  type={f.type === "number" ? "number" : "text"}
                  placeholder={f.type === "json" ? '{"sub": 1}' : "Value"}
                  value={f.value}
                  onChange={(e) => updateField(f.id, { value: e.target.value })}
                  className="flex-1 rounded-lg border border-white/10 bg-black/50 px-2.5 py-1 text-xs text-white outline-none focus:border-purple-500 font-mono"
                />
              )}
              <button
                type="button"
                onClick={() => removeField(f.id)}
                className="rounded-lg p-1 text-slate-500 hover:text-red-400"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
