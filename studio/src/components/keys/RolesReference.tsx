import { Card } from "../ui/card";
import { TokenRoleBadge } from "./TokenRoleBadge";

// Mirrors methodRoles in internal/api/grpc_auth.go (deny by default).
const ROLES: { role: string; can: string; methods: string }[] = [
  { role: "viewer", can: "Read data", methods: "Get, Scan" },
  { role: "writer", can: "Read and write data", methods: "Get, Scan, Put, Delete, BatchWrite" },
  { role: "replication", can: "Follow the WAL as a replica", methods: "StreamWAL, Ack" },
  { role: "admin", can: "Everything above", methods: "All gRPC methods listed here" },
];

/** Four role cards (Figma: API Keys & Roles). Any gRPC method not listed for a role is denied. */
export function RoleCards() {
  return (
    <ul aria-label="Roles" className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {ROLES.map((r) => (
        <li key={r.role}>
          <Card className="h-full px-5 py-4">
            <TokenRoleBadge role={r.role} />
            <p className="mt-3 text-base font-semibold text-foreground">{r.can}</p>
            <p className="mt-1 font-mono text-[11px] text-muted-foreground">{r.methods}</p>
          </Card>
        </li>
      ))}
    </ul>
  );
}

export function RolesNote() {
  return (
    <p className="rounded-lg border border-border bg-muted/40 px-4 py-3 text-xs text-muted-foreground">
      Any gRPC method not listed for a role is denied. The server&apos;s static <code className="font-mono">--api-token</code> always acts as admin.
      Expired tokens are rejected as unauthenticated. Studio sign-in accounts are separate: they are only <span className="font-mono">admin</span> or{" "}
      <span className="font-mono">viewer</span>, and only admins can list, create or revoke keys.
    </p>
  );
}
