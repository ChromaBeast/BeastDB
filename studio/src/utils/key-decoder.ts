export interface KeyDecoded {
  raw: string;
  num: number;
  hex: string;
  prefix: number;
  prefixHex: string;
  prefixLabel: string;
  color: string;
  userHash: number;
  itemHash: number;
}

const PREFIX_META: Record<number, { label: string; color: string }> = {
  0x01: { label: "User Account", color: "emerald" },
  0x02: { label: "User ID Index", color: "teal" },
  0x03: { label: "Game Library", color: "purple" },
  0x04: { label: "Movie Watchlist", color: "cyan" },
  0x05: { label: "Auth Token", color: "amber" },
  0x06: { label: "TV Show Tracker", color: "indigo" },
  0x07: { label: "Book Collection", color: "rose" },
  0x08: { label: "Friendship Graph", color: "blue" },
  0x09: { label: "Friend Request", color: "violet" },
  0x0a: { label: "Username Index", color: "teal" },
  0x0b: { label: "Incoming Requests", color: "sky" },
  0x0c: { label: "Outgoing Requests", color: "sky" },
  0x51: { label: "System Auth / User", color: "fuchsia" },
};

export function decodeKey(key: number | string): KeyDecoded {
  try {
    const bKey = BigInt(key);
    const prefix = Number((bKey >> 56n) & 0xffn);
    const userHash = Number((bKey >> 28n) & 0x0fffffffn);
    const itemHash = Number(bKey & 0x0fffffffn);
    const hex = "0x" + bKey.toString(16).padStart(16, "0");
    const prefixHex = "0x" + prefix.toString(16).padStart(2, "0").toUpperCase();

    const meta = PREFIX_META[prefix] || {
      label: prefix === 0 ? "Default / Root" : `Partition ${prefixHex}`,
      color: "slate",
    };

    return {
      raw: String(key),
      num: Number(bKey),
      hex,
      prefix,
      prefixHex,
      prefixLabel: meta.label,
      color: meta.color,
      userHash,
      itemHash,
    };
  } catch {
    return {
      raw: String(key),
      num: Number(key) || 0,
      hex: "0x0",
      prefix: 0,
      prefixHex: "0x00",
      prefixLabel: "Generic Key",
      color: "slate",
      userHash: 0,
      itemHash: 0,
    };
  }
}

export function formatKeyCompact(key: number | string): string {
  const str = String(key);
  if (str.length > 14) {
    return str.slice(0, 5) + "..." + str.slice(-4);
  }
  return str;
}
