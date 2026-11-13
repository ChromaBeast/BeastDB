import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { TokenRoleBadge } from "./TokenRoleBadge";

// Mirrors methodRoles in internal/api/grpc_auth.go (deny by default).
const ROLES: { role: string; can: string; methods: string }[] = [
  { role: "viewer", can: "Read data", methods: "BeastDBService: Get, Scan" },
  { role: "writer", can: "Read and write data", methods: "Get, Scan, Put, Delete, BatchWrite" },
  { role: "replication", can: "Follow the WAL as a replica", methods: "ReplicationService: StreamWAL, Ack" },
  { role: "admin", can: "Everything above", methods: "All gRPC methods listed here" },
];

export function RolesReference() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Roles</CardTitle>
        <CardDescription>What each token role may call over gRPC. Any method not listed is denied.</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="divide-y divide-border">
          {ROLES.map((r) => (
            <li key={r.role} className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:gap-4">
              <div className="w-28 shrink-0">
                <TokenRoleBadge role={r.role} />
              </div>
              <div className="min-w-0">
                <p className="text-sm text-foreground">{r.can}</p>
                <p className="font-mono text-[11px] text-muted-foreground">{r.methods}</p>
              </div>
            </li>
          ))}
        </ul>
        <p className="mt-4 border-t border-border pt-3 text-xs text-muted-foreground">
          The server&apos;s static <code className="font-mono">--api-token</code> always acts as admin. Expired tokens are rejected as unauthenticated.
          Studio sign-in accounts are separate: they are only <span className="font-mono">admin</span> or <span className="font-mono">viewer</span>, and only
          admins can list, create or revoke keys.
        </p>
      </CardContent>
    </Card>
  );
}
