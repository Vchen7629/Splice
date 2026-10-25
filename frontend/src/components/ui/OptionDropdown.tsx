
interface OptionDropdownProps<T extends string> {
    options: {value: T, label: string}[]
    value: T,
    onChange: (value: T) => void
    ariaLabel: string
    disabled: boolean
}

/**
 * Reusable ui component for shared styling between resolution and denoise select dropdowns
 * @param options - the resolution/denoise quality option list
 * @param value - 
 * @param onChange - the handler to invoke when a resolution/denoise quality is selected
 * @param ariaLabel - for accessibility
 * @param disabled - param passed so users cant select a new value after the job request is submitted
 */
const OptionDropdown = <T extends string>({ options, value, onChange, ariaLabel, disabled }: OptionDropdownProps<T>) => (
    <div className="relative flex items-center">
        <select
            value={value}
            onChange={e => onChange(e.target.value as T)}
            disabled={disabled}
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