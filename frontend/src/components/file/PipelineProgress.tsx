import type { ProcessingType, UploadedFile } from '../../types/file'
import { STATUS_BG } from './StatusStyles'

type SegmentState = 'done' | 'active' | 'pending'

interface Segment {
    key: string
    label: string
    state: SegmentState
    fill: number
}

/**
 * The backend reports a real `stage` per job, and file-upscaling additionally reports
 * a frame-level `progress`. Both pipelines run detect → work → recombine.
 */
const STAGES: Record<'upscale' | 'default', { key: string; label: string }[]> = {
    upscale: [
        { key: 'scene-detector',   label: 'Detect' },
        { key: 'video-upscaling',  label: 'Upscale' },
        { key: 'video-recombiner', label: 'Recombine' },
    ],
    default: [
        { key: 'scene-detector',   label: 'Detect' },
        { key: 'transcoder',       label: 'Transcode' },
        { key: 'video-recombiner', label: 'Recombine' },
    ],
}

/** Maps a file's status/stage to each pipeline segment's visual state and fill %. */
function pipelineSegments(file: UploadedFile, processingType: ProcessingType): Segment[] {
    const stages = STAGES[processingType === 'Upscale' ? 'upscale' : 'default']

    if (file.status === 'complete') {
        return stages.map(s => ({ ...s, state: 'done' as const, fill: 100 }))
    }

    if (file.status === 'uploading' || file.status === 'pending') {
        return stages.map(s => ({ ...s, state: 'pending' as const, fill: 0 }))
    }

    const activeIdx = stages.findIndex(s => s.key === file.stage)

    return stages.map((stage, i) => {
        if (activeIdx === -1) return { ...stage, state: 'pending' as const, fill: 0 }
        if (i < activeIdx) return { ...stage, state: 'done' as const, fill: 100 }
        if (i > activeIdx) return { ...stage, state: 'pending' as const, fill: 0 }

        // Only video-upscaling reports intra-stage progress; everything else shows an
        // indeterminate half-fill rather than a number the backend never sent. Default
        // video-upscaling to 0 (not 50) while waiting for its first progress poll, so
        // the bar doesn't jump to a fake midpoint and then snap back down.
        const fill = stage.key === 'video-upscaling'
            ? file.jobProgress ?? 0
            : 50

        return { ...stage, state: 'active' as const, fill }
    })
}

/**
 * Three-segment track replacing a flat progress bar. Each segment is a real backend
 * stage, so the bar says *where* a job is, not just how far along it is.
 */
const PipelineProgress = ({
    file,
    processingType,
    showLabels = true,
}: {
    file: UploadedFile
    processingType: ProcessingType
    showLabels?: boolean
}) => {
    const segments = pipelineSegments(file, processingType)
    const fillClass = STATUS_BG[file.status]

    if (file.status === 'uploading') {
        return (
            <div className="flex flex-col gap-1">
                <div className="h-[3px] w-full rounded-full bg-track overflow-hidden">
                    <div
                        className="h-full rounded-full bg-status-upload transition-all duration-300"
                        style={{ width: `${file.uploadProgress}%` }}
                    />
                </div>
                {showLabels && (
                    <span className="font-mono text-eyebrow uppercase text-fg-faint">
                        Uploading {file.uploadProgress}%
                    </span>
                )}
            </div>
        )
    }

    return (
        <div className="flex flex-col gap-1">
            <div className="flex items-center gap-[3px]">
                {segments.map(segment => (
                    <div key={segment.key} className="flex-1 h-[3px] rounded-full bg-track overflow-hidden">
                        <div
                            className={`h-full rounded-full transition-all duration-500 ${fillClass}
                                ${segment.state === 'active' ? 'animate-pulse' : ''}`}
                            style={{ width: `${segment.fill}%` }}
                        />
                    </div>
                ))}
            </div>

            {showLabels && (
                <div className="flex items-center gap-[3px]">
                    {segments.map(segment => (
                        <span
                            key={segment.key}
                            className={`flex-1 font-mono text-eyebrow uppercase transition-colors duration-200
                                ${segment.state === 'pending' ? 'text-fg-faint/60' : ''}
                                ${segment.state === 'done' ? 'text-fg-muted' : ''}
                                ${segment.state === 'active' ? 'text-fg-strong' : ''}`}
                        >
                            {segment.label}
                        </span>
                    ))}
                </div>
            )}
        </div>
    )
}

export default PipelineProgress
