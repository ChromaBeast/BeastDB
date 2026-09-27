import { fieldLabel } from "../utils/display";

export function RecordValue({ value }: { value: unknown }) {
  if (value === null)
    return <span className="text-muted-foreground">null</span>;
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="text-muted-foreground">None</span>;
    if (value.every((item) => item === null || typeof item !== "object"))
      return <span className="break-words">{value.map((item) => String(item ?? "null")).join(", ")}</span>;
    return (
      <ul className="space-y-2">
        {value.map((item, index) => <li key={index}><RecordValue value={item} /></li>)}
      </ul>
    );
  }
  if (typeof value === "object")
    return (
      <dl className="divide-y divide-border/50 rounded-md border border-border/70 px-3">
        {Object.entries(value).map(([key, child]) => (
          <div key={key} className="grid min-w-0 grid-cols-[minmax(6rem,32%)_minmax(0,1fr)] gap-3 py-2 text-left">
            <dt className="text-xs text-muted-foreground">{fieldLabel(key)}</dt>
            <dd className="min-w-0 break-words text-sm"><RecordValue value={child} /></dd>
          </div>
        ))}
      </dl>
    );
  if (typeof value === "boolean")
    return <span>{value ? "Yes" : "No"}</span>;
  if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/.test(value)) {
    const date = new Date(value);
    if (!Number.isNaN(date.getTime()))
      return <time dateTime={value} title={value}>{date.toLocaleString()}</time>;
  }
  if (typeof value === "string" && /^https?:\/\//.test(value))
    return (
      <div className="space-y-2">
        {/\.(png|jpe?g|webp|gif|svg)(\?|$)/i.test(value) && (
          <img
            src={value}
            alt="Record field preview"
            className="max-h-40 rounded-md border object-contain"
          />
        )}
        <a
          className="break-all underline underline-offset-2"
          href={value}
          target="_blank"
          rel="noreferrer"
        >
          {value}
        </a>
      </div>
    );
  return <span className="whitespace-pre-wrap break-words">{String(value)}</span>;
}
