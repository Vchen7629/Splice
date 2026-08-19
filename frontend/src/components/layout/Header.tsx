import { Moon, Sun } from 'lucide-react'
import type { Mode } from '../../hooks/useTheme'
import type { ReactNode } from 'react'

interface HeaderProps {
    nav: ReactNode
    darkLightMode: Mode
    onToggleMode: () => void
}

const Header = ({ nav, darkLightMode, onToggleMode }: HeaderProps) => (
    <header className="relative flex items-end gap-8 h-14 shrink-0 px-6 border-b border-line bg-header-bg backdrop-blur-sm">
        {/* the logo + site name */}
        <span className="flex items-center gap-2 select-none shrink-0 mb-3">
            <svg width="13" height="15" viewBox="0 0 13 15" aria-hidden="true">
                <rect x="0" y="0" width="5" height="11" rx="1" fill="currentColor" className="text-fg-strong" />
                <rect x="8" y="4" width="5" height="11" rx="1" fill="currentColor" className="text-fg-faint" />
            </svg>
            <span className="text-title font-bold text-fg-strong">Splice</span>
        </span>

        {/* nav buttons + dark light mode button */}
        <nav className="flex items-end gap-7 h-full">{nav}</nav>
        <div className="ml-auto mb-3 flex items-center gap-1.5">
            <IconButton
                label={darkLightMode === 'light' ? 'Switch to dark mode' : 'Switch to light mode'}
                onClick={onToggleMode}
            >
                {darkLightMode === 'light' ? <Moon size={15} /> : <Sun size={15} />}
            </IconButton>
        </div>
    </header>
)

const IconButton = ({
    label, onClick, children,
}: { label: string; onClick?: () => void; children: React.ReactNode }) => (
    <button
        onClick={onClick}
        aria-label={label}
        title={label}
        className="flex items-center justify-center w-8 h-8 rounded-lg border border-line text-fg-muted
            hover:text-fg-strong hover:border-line-strong hover:bg-row transition-colors duration-100"
    >
        {children}
    </button>
)

export default Header
