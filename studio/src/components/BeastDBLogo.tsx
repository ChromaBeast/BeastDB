export function BeastDBLogo() {
  return (
    <div className="shrink-0 flex items-center" role="img" aria-label="BeastDB">
      <img
        src="/brand/beastdb-horizontal-light.svg"
        alt="BeastDB"
        aria-hidden="true"
        className="h-8 w-auto dark:hidden"
      />
      <img
        src="/brand/beastdb-horizontal-dark.svg"
        alt="BeastDB"
        aria-hidden="true"
        className="hidden h-8 w-auto dark:block"
      />
      <span className="ml-2.5 rounded border border-zinc-800 bg-zinc-900/80 px-1.5 py-0.5 text-[10px] font-mono text-zinc-400">
        Studio
      </span>
    </div>
  );
}
