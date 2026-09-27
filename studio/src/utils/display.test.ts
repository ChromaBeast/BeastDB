import { expect, test } from "bun:test";
import { parseUniversalRecord } from "./data-parser";
import { fieldLabel, recordPreview } from "./display";

test("movie activity uses readable content instead of a partition code", () => {
  const record = parseUniversalRecord({
    keyText: "299902639426514427",
    value: JSON.stringify({
      userId: "u-Clara-2",
      movieId: "m-6",
      status: "completed",
      userRating: 5,
      notes: "Unmatched animation style.",
      addedAt: "2026-09-25T20:33:59.29658317Z",
    }),
  });

  expect(record.primaryLabel).toBe("Unmatched animation style.");
  expect(recordPreview(record)).toBe("completed");
  expect(record.keyStr).toBe("299902639426514427");
  expect(fieldLabel("userRating")).toBe("User Rating");
  expect(fieldLabel("movieId")).toBe("Movie ID");
});

test("unnamed documents retain a useful fallback", () => {
  const record = parseUniversalRecord({ keyText: "1", value: "{}" });
  expect(record.primaryLabel).toBe("Record 1");
});

test("nested titles work for any object name", () => {
  const record = parseUniversalRecord({
    keyText: "218374265768618879",
    value: JSON.stringify({
      itemId: "steam-291550",
      status: "playing",
      notes: "Synced from Steam",
      catalogEntry: { id: "steam-291550", title: "Brawlhalla", genres: ["Steam"] },
    }),
  });

  expect(record.primaryLabel).toBe("Brawlhalla");
  expect(recordPreview(record)).toBe("playing · Synced from Steam");
});
