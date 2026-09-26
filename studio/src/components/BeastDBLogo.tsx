export function BeastDBLogo() {
  return (
    <div className="shrink-0" role="img" aria-label="BeastDB">
      <img
        src="/brand/beastdb-horizontal-light.svg"
        alt=""
        aria-hidden="true"
        className="h-9 w-auto dark:hidden"
      />
      <img
        src="/brand/beastdb-horizontal-dark.svg"
        alt=""
        aria-hidden="true"
        className="hidden h-9 w-auto dark:block"
      />
    </div>
  );
}
