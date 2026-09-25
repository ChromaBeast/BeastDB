"use client";

import React, { useState } from "react";
import { X, Sparkles, Code2, Plus } from "lucide-react";
import { fnv1a64 } from "../utils/format";
import { NewRecordVisualForm } from "./NewRecordVisualForm";
import { NewRecordRawForm } from "./NewRecordRawForm";

interface NewRecordModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (key: number, value: string) => Promise<boolean>;
}

export const NewRecordModal: React.FC<NewRecordModalProps> = ({ isOpen, onClose, onSave }) => {
  const [tab, setTab] = useState<"visual" | "raw">("visual");
  const [type, setType] = useState("game");
  const [title, setTitle] = useState("");
  const [status, setStatus] = useState("Planned");
  const [rating, setRating] = useState("8.5");
  const [description, setDescription] = useState("");
  const [customKey, setCustomKey] = useState("");
  const [rawKey, setRawKey] = useState("");
  const [rawValue, setRawValue] = useState('{\n  "title": "Example Record",\n  "type": "game"\n}');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    let keyToSave: number;
    let valueToSave: string;

    if (tab === "visual") {
      if (!title.trim()) {
        alert("Please enter a title or name");
        setIsSubmitting(false);
        return;
      }
      keyToSave = customKey.trim() ? Number(customKey) : fnv1a64(`${type}:${title.trim()}`);
      valueToSave = JSON.stringify({
        title: title.trim(),
        type,
        status,
        rating: Number(rating) || rating,
        description: description.trim(),
        created_at: new Date().toISOString(),
      });
    } else {
      keyToSave = Number(rawKey);
      if (!keyToSave || isNaN(keyToSave)) {
        alert("Please enter a valid numeric 64-bit key");
        setIsSubmitting(false);
        return;
      }
      valueToSave = rawValue;
    }

    const success = await onSave(keyToSave, valueToSave);
    setIsSubmitting(false);
    if (success) {
      setTitle("");
      setDescription("");
      setCustomKey("");
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-[#0d121f] p-6 shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] pb-3">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-600/20 text-purple-400">
              <Plus className="h-4 w-4" />
            </div>
            <h2 className="text-sm font-semibold text-white">Create New BeastDB Record</h2>
          </div>
          <button onClick={onClose} className="rounded-lg p-1 text-slate-400 hover:text-white">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="my-4 flex rounded-xl border border-white/[0.08] bg-black/30 p-1">
          <button
            type="button"
            onClick={() => setTab("visual")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium ${
              tab === "visual" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
          >
            <Sparkles className="h-3.5 w-3.5" /> Friendly Builder
          </button>
          <button
            type="button"
            onClick={() => setTab("raw")}
            className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-xs font-medium ${
              tab === "raw" ? "bg-purple-600/30 text-purple-300" : "text-slate-400 hover:text-white"
            }`}
          >
            <Code2 className="h-3.5 w-3.5" /> Raw Key-Value
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3">
          {tab === "visual" ? (
            <NewRecordVisualForm
              type={type}
              setType={setType}
              status={status}
              setStatus={setStatus}
              title={title}
              setTitle={setTitle}
              rating={rating}
              setRating={setRating}
              customKey={customKey}
              setCustomKey={setCustomKey}
              description={description}
              setDescription={setDescription}
            />
          ) : (
            <NewRecordRawForm
              rawKey={rawKey}
              setRawKey={setRawKey}
              rawValue={rawValue}
              setRawValue={setRawValue}
            />
          )}

          <div className="flex items-center justify-end gap-2 pt-3 border-t border-white/[0.08]">
            <button
              type="button"
              onClick={onClose}
              className="rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-xs font-medium text-slate-300 hover:text-white"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="rounded-xl bg-purple-600 px-5 py-2 text-xs font-semibold text-white shadow-lg shadow-purple-600/30 hover:bg-purple-500 disabled:opacity-50"
            >
              {isSubmitting ? "Writing to B+ Tree..." : "Insert Record"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
