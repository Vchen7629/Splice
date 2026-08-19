import type { ReactNode } from "react"

interface SidebarProps {
    queueCount: number
    queueContent: ReactNode
    outputCount: number
    outputContent: ReactNode
}

/**2 stacked sections with sticky headers (processing and done videos) */
const Sidebar = ({ queueCount, queueContent, outputCount, outputContent }: SidebarProps) => (
    <aside className="flex flex-col w-[340px] shrink-0 border-l border-line bg-panel">
        <section className="flex flex-col flex-1 min-h-0">
            <SectionHeader label="Queue" count={queueCount}/>
            {queueContent}
        </section>

        <section className="flex flex-col shrink-0 max-h-[45%] border-t border-line">
            <SectionHeader label="Output" count={outputCount}/>
            {outputContent}
        </section>
    </aside>
)

const SectionHeader = ({ label, count }: { label: string; count: number }) => (
    <div className="flex items-center gap-2 px-5 h-10 shrink-0 border-b border-line bg-panel sticky top-0 z-10">
        <span className="font-mono text-eyebrow font-semibold uppercase text-fg-muted">{label}</span>
        <span className="ml-auto font-mono text-eyebrow text-fg-faint tabular-nums">
            {count.toString().padStart(2, '0')}
        </span>
    </div>
)

export default Sidebar