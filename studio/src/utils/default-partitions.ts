export interface PartitionMeta {
  label: string;
  color: string;
  description?: string;
}

export const DEFAULT_REGISTRY: Record<number, PartitionMeta> = {
  0x01: { label: "User Accounts", color: "emerald", description: "User profiles and authentication credentials" },
  0x02: { label: "User by ID", color: "teal", description: "Secondary index mapping user UUIDs to emails" },
  0x03: { label: "Game Collection", color: "purple", description: "User game library entries and progress" },
  0x04: { label: "Movie Collection", color: "cyan", description: "User movie library entries and watchlist" },
  0x05: { label: "Refresh Tokens", color: "amber", description: "Hashed session and API tokens" },
  0x06: { label: "TV Collection", color: "indigo", description: "User TV show progress and episode tracking" },
  0x07: { label: "Book Collection", color: "rose", description: "User reading status and page progress" },
  0x08: { label: "Friendships", color: "blue", description: "Bidirectional social graph edges" },
  0x09: { label: "Friend Requests", color: "violet", description: "Pending and accepted friend requests" },
  0x0a: { label: "User by Username", color: "teal", description: "Secondary index mapping usernames to emails" },
  0x0b: { label: "Inbox Requests", color: "sky", description: "Incoming friend requests by receiver" },
  0x0c: { label: "Outbox Requests", color: "sky", description: "Outgoing friend requests by sender" },
};
