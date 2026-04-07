const Header = () => {
  const usedGb = 12.4
  const totalGb = 50
  const pct = (usedGb / totalGb) * 100

  return (
    <header className="sticky flex bg-header-bg items-center justify-between w-full shrink-0 px-6 h-[60px] border-b-1 border-line z-10 top-0 backdrop-blur-[12px]">
      <span className="text-sm tracking-[0.2em] select-none font-mono font-medium text-zinc-300">
        splice
      </span>

      <div className="flex items-center gap-5">

        {/* Storage quota */}
        <div className="flex items-center gap-2.5">
          <div className="relative rounded-full overflow-hidden w-[52px] h-[3px] bg-progress">
            <div
              className="absolute left-0 top-0 h-full rounded-full bg-accent"
              style={{
                width: `${pct}%`,
              }}
            />
          </div>
          <span className="text-xs tabular-nums font-mono text-text-1 tracking-normal">
            {usedGb} / {totalGb} GB
          </span>
        </div>

        <div className="h-[16px] border-[0.5px] border-border-1"/>

        <span className="text-xs text-text-1">
          user@example.com
        </span>

        <button
          className="text-xs px-3 py-1.5 rounded-md text-white bg-emerald-800 border border-emerald-900 hover:bg-emerald-700 transition-colors"
        >
          Sign out
        </button>
      </div>
    </header>
  )
}

export default Header
