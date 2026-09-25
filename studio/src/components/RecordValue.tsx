export function RecordValue({ value }: { value: unknown }) {
  if (value === null)
    return <span className="text-muted-foreground">null</span>;
  if (typeof value === "object")
    return (
      <div className="ml-2 space-y-2 border-l pl-3">
        {Object.entries(value).map(([key, child]) => (
          <div key={key}>
            <span className="font-mono text-xs text-muted-foreground">
              {key}
            </span>
            <div className="break-all text-sm">
              <RecordValue value={child} />
            </div>
          </div>
        ))}
      </div>
    );
  if (typeof value === "boolean")
    return <span>{value ? "true" : "false"}</span>;
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
  return <span className="whitespace-pre-wrap break-all">{String(value)}</span>;
}
