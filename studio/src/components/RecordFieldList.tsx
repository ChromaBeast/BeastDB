import { fieldLabel } from "../utils/display";
import { RecordValue } from "./RecordValue";

function isIdentifier(name: string): boolean {
  return name === "id" || name.endsWith("Id") || /(?:^|[_-])id$/i.test(name);
}

export function RecordFieldList({ fields, identifiers = false }: {
  fields: Record<string, unknown>;
  identifiers?: boolean;
}) {
  const entries = Object.entries(fields).filter(([name]) => isIdentifier(name) === identifiers);
  if (entries.length === 0) return null;

  return (
    <dl className="divide-y divide-border/60 rounded-lg border border-border bg-card px-4">
      {entries.map(([name, value]) => {
        const complex = value !== null && typeof value === "object";
        return (
          <div key={name} className={complex ? "py-3" : "grid grid-cols-[minmax(7rem,30%)_minmax(0,1fr)] gap-3 py-3"}>
            <dt className="text-xs text-muted-foreground">{fieldLabel(name)}</dt>
            <dd className={complex ? "mt-2 min-w-0" : "min-w-0 break-words text-sm"}>
              <RecordValue value={value} />
            </dd>
          </div>
        );
      })}
    </dl>
  );
}
