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
const STAGES: Record<'Upscale' | 'Transcode' | 'Denoise' | 'Convert', { key: string; label: string }[]> = {
    Upscale: [
        { key: 'scene-detector',   label: 'Detect' },
        { key: 'video-upscaling',  label: 'Upscale' },
        { key: 'video-recombiner', label: 'Recombine' },
    ],
    Transcode: [
        { key: 'scene-detector',   label: 'Detect' },
        { key: 'transcoder',       label: 'Transcode' },
        { key: 'video-recombiner', label: 'Recombine' },
    ],
    Denoise: [
        { key: 'scene-detector',   label: 'Detect' },
        { key: 'denoiser',         label: 'Denoise' },
        { key: 'video-recombiner', label: 'Recombine' },
    ],
    Convert: [
        { key: 'converter',        label: 'Convert' },
    ],
}

const UPLOAD_STEP = { key: 'upload', label: 'Upload'}

/** Maps a file's status/stage to each pipeline segment's visual state and fill %. */
function pipelineSegments(file: UploadedFile, processingType: ProcessingType): Segment[] {
    const steps = [UPLOAD_STEP, ...STAGES[processingType]]

    if (file.status === 'complete') {
        return steps.map(s => ({ ...s, state: 'done' as const, fill: 100 }))
    }

    if (file.status === 'pending') {
        return steps.map(s => ({ ...s, state: 'pending' as const, fill: 0 }))
    }

    if (file.status === 'uploading') {
        return steps.map((s, i) => i === 0 
            ? { ...s, state: 'active' as const, fill: file.uploadProgress }
            : { ...s, state: 'pending' as const, fill: 0})
    }

    // Processing: upload is done, find the active backend stage among the rest.
    const activeIdx = 1 + steps.slice(1).findIndex(s => s.key === file.stage)

    return steps.map((step, i) => {
        if (i === 0) return { ...step, state: 'done' as const, fill: 100 }
        if (activeIdx === 0) return { ...step, state: 'pending' as const, fill: 0 }
        if (i < activeIdx) return { ...step, state: 'done' as const, fill: 100 }
        if (i > activeIdx) return { ...step, state: 'pending' as const, fill: 0 }

        // TODO: update this in the future to show granular progress
        // Only video-upscaling reports intra-stage progress; everything else shows an
        // indeterminate half-fill rather than a number the backend never sent. Default
        // video-upscaling to 0 (not 50) while waiting for its first progress poll, so
        // the bar doesn't jump to a fake midpoint and then snap back down.
        const fill = step.key === 'video-upscaling' ? file.jobProgress ?? 0 : 50

        return { ...step, state: 'active' as const, fill }
    })
}

/**
 * 4 segment track with each segment showing where the progress for the current job is
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
