"use client";

import React from "react";

interface NewRecordVisualFormProps {
  type: string;
  setType: (v: string) => void;
  status: string;
  setStatus: (v: string) => void;
  title: string;
  setTitle: (v: string) => void;
  rating: string;
  setRating: (v: string) => void;
  customKey: string;
  setCustomKey: (v: string) => void;
  description: string;
  setDescription: (v: string) => void;
}

export const NewRecordVisualForm: React.FC<NewRecordVisualFormProps> = ({
  type,
  setType,
  status,
  setStatus,
  title,
  setTitle,
  rating,
  setRating,
  customKey,
  setCustomKey,
  description,
  setDescription,
}) => {
  return (
    <>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-[11px] font-medium text-slate-400">Category Type</label>
          <select
            value={type}
            onChange={(e) => setType(e.target.value)}
            className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500"
          >
            <option value="game">Game</option>
            <option value="movie">Movie</option>
            <option value="tv">TV Show</option>
            <option value="book">Book</option>
            <option value="custom">Custom Entity</option>
          </select>
        </div>
        <div>
          <label className="text-[11px] font-medium text-slate-400">Status</label>
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500"
          >
            <option value="Planned">Planned</option>
            <option value="In Progress">In Progress</option>
            <option value="Completed">Completed</option>
            <option value="On Hold">On Hold</option>
          </select>
        </div>
      </div>

      <div>
        <label className="text-[11px] font-medium text-slate-400">Title / Entity Name *</label>
        <input
          type="text"
          required
          placeholder="e.g. Elden Ring, Interstellar"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500"
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-[11px] font-medium text-slate-400">Rating (0-10)</label>
          <input
            type="number"
            step="0.1"
            min="0"
            max="10"
            value={rating}
            onChange={(e) => setRating(e.target.value)}
            className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500"
          />
        </div>
        <div>
          <label className="text-[11px] font-medium text-slate-400">Custom Key (optional)</label>
          <input
            type="number"
            placeholder="Auto FNV-1a if blank"
            value={customKey}
            onChange={(e) => setCustomKey(e.target.value)}
            className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 px-3 py-1.5 text-xs text-white outline-none focus:border-purple-500"
          />
        </div>
      </div>

      <div>
        <label className="text-[11px] font-medium text-slate-400">Description / Summary</label>
        <textarea
          rows={3}
          placeholder="Short overview of the entity..."
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          className="mt-1 w-full rounded-xl border border-white/10 bg-black/40 p-2.5 text-xs text-white outline-none focus:border-purple-500 resize-none"
        />
      </div>
    </>
  );
};
