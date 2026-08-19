import type { ProcessingType, UploadedFile } from '../../types/file'
import { TRANSCODE_RESOLUTIONS, UPSCALE_RESOLUTIONS, type Resolution } from '../../utils/videoResolution'

interface ResolutionSelectProps {
    processingType: ProcessingType
    file: UploadedFile
    handleSetResolution: (id: number, resolution: string) => void
}

/** Upscaling only offers targets above the source height, so the control never lies. */
function resolutionOptions(processingType: ProcessingType, sourceHeight: number): Resolution[] {
    if (processingType !== 'Upscale') return TRANSCODE_RESOLUTIONS

    const larger = UPSCALE_RESOLUTIONS.filter(r => r.height > sourceHeight)
    return larger.length > 0 ? larger : UPSCALE_RESOLUTIONS
}

const ResolutionSelect = ({ processingType, file, handleSetResolution }: ResolutionSelectProps) => (
    <div className="relative flex items-center">
        <select
            value={file.resolution}
            onChange={e => handleSetResolution(file.id, e.target.value)}
            aria-label={`Target resolution for ${file.name}`}
            className="resolution-select font-mono text-meta text-fg bg-input-bg border border-line rounded px-1.5 h-[22px] w-[68px] outline-none cursor-pointer hover:border-line-strong transition-colors duration-100"
        >
            {resolutionOptions(processingType, file.sourceHeight).map(r => (
                <option key={r.label} value={r.label}>{r.label}</option>
            ))}
        </select>
        <span className="select-arrow absolute right-1.5 pointer-events-none text-fg-muted" aria-hidden="true" />
    </div>
)

export default ResolutionSelect
