import type { UploadedVideo } from "../types/video"

const STAGE_PROGRESS: Record<string, number> = {
    '': 5,
    'scene-detector': 33,
    'transcoder': 77,
    'video-recombiner': 90,
}

const STAGE_LABELS: Record<string, string> = {
    '': 'Queued',
    'scene-detector': 'Detecting scenes',
    'transcoder': 'Transcoding',
    'video-recombiner': 'Recombining',
}

const DEGRADED_LABELS: Record<string, string> = {
    'upload': 'Degraded: scene-detector is down',
    'scene-detector': 'Degraded: transcoder is down',
    'transcoder': 'Degraded: video-recombiner is down',
}

const VideoProgressBar = ({ video }: { video: UploadedVideo }) => {
    const stage = video.stage ?? ''
    const pct = STAGE_PROGRESS[stage] ?? 5
    const label = video.status === 'degraded' ? (DEGRADED_LABELS[stage] ?? 'Degraded, backend service is down...')
        : video.status === 'error' ? 'Failed'
        : (STAGE_LABELS[stage] ?? stage)

    let barColor = 'bg-green-500'
    if (video.status === 'degraded') barColor = 'bg-orange-400'
    else if (video.status === 'error') barColor = 'bg-red-500'

    return (
        <div className="mt-1.5">
            <span className="text-[10px] font-mono text-zinc-400">{label}</span>
            <div className="h-1 w-full rounded-full bg-zinc-700 mt-0.5">
                <div
                    className={`h-1 rounded-full transition-all duration-500 ${barColor}`}
                    style={{ width: `${pct}%` }}
                />
            </div>
        </div>
    )
}

export default VideoProgressBar