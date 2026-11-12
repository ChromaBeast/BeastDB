// 64-bit key rendered as a bit matrix: 8 prefix bits, 28 domain bits, 28 item bits.
// A lit cell is a 1 bit; the prefix (partition) is accent-coloured.
const COLS = 32;
const CELL = 3;
const GAP = 1;

export function KeyStrip({ keyStr, className }: { keyStr: string; className?: string }) {
  let bits: string;
  try {
    bits = BigInt(keyStr).toString(2).padStart(64, "0");
  } catch {
    return null;
  }
  const step = CELL + GAP;
  const w = COLS * step - GAP;
  const h = 2 * step - GAP;
  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      className={className}
      role="img"
      aria-label="64-bit key: 8 partition bits, 28 domain bits, 28 item bits"
    >
      {Array.from(bits).map((b, i) => {
        const on = b === "1";
        const cls = i < 8 ? (on ? "fill-primary" : "fill-primary/20") : on ? (i < 36 ? "fill-foreground/60" : "fill-foreground/35") : "fill-border";
        return <rect key={i} x={(i % COLS) * step} y={Math.floor(i / COLS) * step} width={CELL} height={CELL} rx={0.5} className={cls} />;
      })}
    </svg>
  );
}
