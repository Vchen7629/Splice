import { Clapperboard, Sparkles, Wind, ArrowLeftRight, Settings } from 'lucide-react'
import type { ProcessingType } from '../types/video'

interface SidebarProps {
    activeFeature: ProcessingType
    onSelect: (feature: ProcessingType) => void
}

interface NavEntry {
    feature: ProcessingType
    label: string
    icon: React.ReactNode
}

const NAV_ENTRIES: NavEntry[] = [
    { feature: 'Transcode', label: 'Transcode',  icon: <Clapperboard size={15} /> },
    { feature: 'Upscale',   label: 'Upscale',    icon: <Sparkles size={15} /> },
    { feature: 'Denoise',   label: 'Denoise',    icon: <Wind size={15} /> },
    { feature: 'Convert',   label: 'Convert',    icon: <ArrowLeftRight size={15} /> },
]

const Sidebar = ({ activeFeature, onSelect }: SidebarProps) => {
    return (
        <aside className="flex flex-col w-[200px] shrink-0 bg-panel border-r border-[var(--border)] h-[100vh]">
            <span className="text-[17px] font-semibold select-none text-stone-100 tracking-tight px-4 py-5">
                Splice
            </span>
            {/* Group label */}
            <div className="px-4 pt-5 pb-1">
                <span className="text-[9px] font-semibold tracking-widest uppercase text-stone-600">
                    Process
                </span>
            </div>

            {/* Nav entries */}
            <nav className="flex flex-col flex-1 px-2 gap-0.5">
                {NAV_ENTRIES.map(({ feature, label, icon }) => {
                    const isActive = activeFeature === feature
                    return (
                        <button
                            key={feature}
                            onClick={() => onSelect(feature)}
                            className={`
                                relative flex items-center gap-2.5 h-10 px-3 rounded-[6px] w-full text-left
                                transition-colors duration-100
                                ${isActive
                                    ? 'text-amber-400'
                                    : 'text-stone-400 hover:text-stone-200 hover:bg-stone-700/30'
                                }
                            `}
                        >
                            {/* Active left border strip */}
                            {isActive && (
                                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-[2px] h-5 rounded-full bg-amber-400" />
                            )}
                            <span className="shrink-0">{icon}</span>
                            <span className="text-[12px] font-medium">{label}</span>
                        </button>
                    )
                })}
            </nav>

            {/* Bottom: Settings */}
            <div className="px-2 pb-4">
                <button className="flex items-center gap-2.5 h-10 px-3 rounded-[6px] w-full text-left text-stone-500 hover:text-stone-300 hover:bg-stone-700/30 transition-colors duration-100">
                    <Settings size={15} />
                    <span className="text-[12px] font-medium">Settings</span>
                </button>
            </div>
        </aside>
    )
}

export default Sidebar
