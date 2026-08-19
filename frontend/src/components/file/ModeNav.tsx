import type { ProcessingType } from "../../types/file";

const MODES: ProcessingType[] = ['Transcode', 'Upscale', 'Denoise', 'Convert']

interface ModeNavProps {
    activeFeature: ProcessingType
    onSelectFeature: (feature: ProcessingType) => void
}

/** the nav buttons in header to switch between transcode/upscale/denoise etc*/
const ModeNav = ({ activeFeature, onSelectFeature }: ModeNavProps) => (
    <>
        {MODES.map(feature => {
            const isActive = activeFeature === feature
            return (
                <button
                    key={feature}
                    onClick={() => onSelectFeature(feature)}
                    aria-current={isActive ? 'page' : undefined}
                    className={`relative flex items-center h-full px-0.5 text-caption tracking-wide transition-colors duration-100
                          ${isActive ? 'text-accent font-semibold' : 'text-fg-muted font-medium hover:text-fg-strong'}`}
                >
                    {feature}
                    {isActive && <span className="absolute left-0 right-0 -bottom-px h-[2.5px] rounded-t-full bg-accent"/>}
                </button>
            )
        })}
    </>
)

export default ModeNav