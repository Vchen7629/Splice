
interface OptionDropdownProps {
    options: {value: string, label: string}[]
    value: string,
    onChange: (value: string) => void
    ariaLabel: string
}

/**
 * Reusable ui component for shared styling between resolution and denoise select dropdowns
 * @param options - the resolution/denoise quality option list
 * @param value - 
 * @param onChange - the handler to invoke when a resolution/denoise quality is selected
 * @param ariaLabel - for accessibility
 */
const OptionDropdown = ({ options, value, onChange, ariaLabel }: OptionDropdownProps) => (
    <div className="relative flex items-center">
        <select
            value={value}
            onChange={e => onChange(e.target.value)}
            aria-label={ariaLabel}
            className="resolution-select font-mono text-meta text-fg bg-input-bg border border-line rounded px-1.5 h-[22px] w-[68px] outline-none cursor-pointer hover:border-line-strong transition-colors duration-100"
        >
            {options.map(o => (
                <option key={o.label} value={o.value}>{o.label}</option>
            ))}
        </select>
        <span className="select-arrow absolute right-1.5 pointer-events-none text-fg-muted" aria-hidden="true" />
    </div>
)

export default OptionDropdown