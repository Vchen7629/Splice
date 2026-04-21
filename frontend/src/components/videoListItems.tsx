import type { JobStatus, ProcessingType, UploadedVideo } from "../types/video"
import { TRANSCODE_RESOLUTIONS, UPSCALE_RESOLUTIONS, type Resolution } from "../utils/videoResolution"

interface ResolutionSelectProps {
    processingType: ProcessingType
    video: UploadedVideo
    handleSetResolution: (id: number, resolution: string) => void
}

const ResolutionSelect = ({ processingType, video, handleSetResolution }: ResolutionSelectProps) => {
    let options: Resolution[]

    if (processingType === 'Upscale') {
        options = UPSCALE_RESOLUTIONS.filter(r => r.height > video.sourceHeight)
        if (options.length === 0) options = UPSCALE_RESOLUTIONS
    } else {
        options = TRANSCODE_RESOLUTIONS
    }

    return (
        <div className="ml-auto">
            <select
                value={video.resolution}
                onChange={e => handleSetResolution(video.id, e.target.value)}
                className="resolution-select font-mono text-[10px] text-stone-300 bg-[var(--bg-input)] border border-[var(--border)] rounded px-1.5 h-[20px] w-[64px] outline-none cursor-pointer hover:border-[var(--border-1)] transition-colors duration-100"
            >
                {options.map((r: Resolution) => <option key={r.label} value={r.label}>{r.label}</option>)}
            </select>
        </div>
    )
}

const PROGRESS_BAR: Record<JobStatus, string> = {
    pending:    'bg-stone-600',
    uploading:  'bg-sky-400',
    processing: 'bg-green-400',
    complete:   'bg-green-400',
    error:      'bg-red-500',
    degraded:   'bg-orange-400',
}

const STAGE_PROGRESS: Record<string, number> = {
    '':                 5,
    'scene-detector':   33,
    'transcoder':       77,
    'video-recombiner': 90,
}

const STAGE_LABELS: Record<string, string> = {
    '':                 'Queued',
    'scene-detector':   'Detecting scenes',
    'transcoder':       'Transcoding',
    'video-recombiner': 'Recombining',
}

/**
 * Progress bar of an uploaded video that is processing. Shown in uploadVideoList component.
 */
const VideoProgressBar = ({ video }: { video: UploadedVideo }) => {
    function getProgress(video: UploadedVideo): { pct: number; label: string } {
        if (video.status === 'uploading') {
            return { pct: video.uploadProgress, label: `${video.uploadProgress}%` }
        }

        const stage = video.stage ?? ''
        const pct = STAGE_PROGRESS[stage] ?? 5

        if (video.status === 'error')    return { pct: 100, label: 'Failed' }
        if (video.status === 'degraded') return { pct, label: 'Service degraded' }

        return { pct, label: STAGE_LABELS[stage] ?? stage }
    }

    const { pct } = getProgress(video)

    return (
        <div className="pt-0.5 h-[2px] w-full rounded-full bg-[var(--bg-progress)]">
            <div
                className={`h-full rounded-full transition-all duration-500 ${PROGRESS_BAR[video.status]}`}
                style={{ width: `${pct}%` }}
            />
        </div>
    )
}

/**
 * Displays the video name and truncates it and replaces the extra length with ... if too long
 * @param video - the video we are displaying name for
 */
const VideoName = ({ video }: { video: UploadedVideo }) => {
    function truncateName(name: string): string {
        const dotIdx = name.lastIndexOf('.')
        const ext  = dotIdx !== -1 ? name.slice(dotIdx) : ''
        const base = dotIdx !== -1 ? name.slice(0, dotIdx) : name

        if (base.length <= 18) return name

        return base.slice(0, 14) + '…' + ext
    }

    return (
        <span
            className="flex-1 min-w-0 truncate font-mono text-[12px] text-stone-200 tracking-tight"
            title={video.name}
        >
            {truncateName(video.name)}
        </span>
    )
}

export { ResolutionSelect, VideoName, VideoProgressBar }
