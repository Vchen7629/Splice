import { X } from "lucide-react";
import { useVideoQueueStore } from "../state/videoQueue";
import type { JobStatus, ProcessingType, UploadedVideo } from "../types/video";
import { formatSize } from "../utils/fileDisplay";
import { ResolutionSelect, VideoName, VideoProgressBar } from "./videoListItems";
import { VideoDownloadButton, VideoRemoveButton } from "./videoListButtons";

// Full class strings so Tailwind can detect them at build time
const STATUS_STRIP: Record<JobStatus, string> = {
    pending:    'bg-stone-600',
    uploading:  'bg-sky-400',
    processing: 'bg-amber-400',
    complete:   'bg-green-400',
    error:      'bg-red-500',
    degraded:   'bg-orange-400',
}

const STATUS_LABEL: Record<JobStatus, { text: string; classes: string; pulse: boolean }> = {
    pending:    { text: 'Queued',     classes: 'text-stone-500',  pulse: false },
    uploading:  { text: 'Uploading',  classes: 'text-sky-400',    pulse: true  },
    processing: { text: 'Processing', classes: 'text-amber-400',  pulse: true  },
    complete:   { text: 'Done',       classes: 'text-green-400',  pulse: false },
    error:      { text: 'Failed',     classes: 'text-red-400',    pulse: false },
    degraded:   { text: 'Degraded',   classes: 'text-orange-400', pulse: true  },
}

interface UploadVideoListProps {
    processingType: ProcessingType
    videos: UploadedVideo[]
    onRemove: (processingType: ProcessingType, id: number) => void
    showResolution?: boolean
}

const UploadVideoList = ({ processingType, videos, onRemove, showResolution = true }: UploadVideoListProps) => {
    const { resetVideo, setResolution } = useVideoQueueStore()

    function handleSetResolution(id: number, resolution: string) {
        const video = videos.find(v => v.id === id)

        if (video?.status === 'error') resetVideo(processingType, id)

        setResolution(processingType, id, resolution)
    }

    return (
        <ul className="flex flex-col gap-1.5 p-3 flex-1 overflow-y-auto bg-row-bg">
            {videos.map(video => {
                const status = STATUS_LABEL[video.status]

                return (
                    <li
                        key={video.id}
                        className="flex rounded-[10px] overflow-hidden border border-[var(--border)] hover:border-[var(--border-1)] bg-panel transition-colors duration-150"
                    >
                        {/* Status strip */}
                        <div className={`w-[3px] shrink-0 ${STATUS_STRIP[video.status]}`} />

                        {/* Content */}
                        <div className="flex flex-col flex-1 px-3 py-2 min-w-0 gap-1">
                            {/* Row 1: filename · status · remove */}
                            <div className="flex items-center gap-2">
                                <VideoName video={video}/>

                                <span className={`flex items-center gap-1 font-mono text-[10px] shrink-0 ${status.classes}`}>
                                    {status.pulse && <span className="w-1 h-1 rounded-full bg-current animate-pulse" />}
                                    {status.text}
                                </span>

                                <VideoRemoveButton video={video} processingType={processingType} onRemove={onRemove}/>
                            </div>

                            {/* Row 2: size + resolution */}
                            <div className="flex items-center gap-2">
                                <span className="font-mono text-[10px] text-stone-500 tabular-nums">
                                    {formatSize(video.size)}
                                </span>
                                {showResolution && (
                                    <ResolutionSelect
                                        processingType={processingType}
                                        video={video}
                                        handleSetResolution={handleSetResolution}
                                    />
                                )}
                            </div>

                            <VideoProgressBar video={video}/>
                        </div>
                    </li>
                )
            })}
        </ul>
    )
}


interface processedVideoListProps {
    processingType: ProcessingType
    processedVideos: UploadedVideo[] 
    onRemove: (processingType: ProcessingType, id: number) => void
}

const ProcessedVideoList = ({ processingType, processedVideos, onRemove}: processedVideoListProps) => {
    return (
        <ul className="flex flex-col gap-1.5 p-3 flex-1 overflow-y-auto bg-row-bg">
            {processedVideos.map(video => (
                <li
                    key={video.id}
                    className="flex rounded-[10px] overflow-hidden border border-[var(--border)] hover:border-[var(--border-1)] bg-panel transition-colors duration-150"
                >
                    {/* Status strip — green for completed */}
                    <div className="w-[3px] shrink-0 bg-green-400" />

                    {/* Content */}
                    <div className="flex flex-col flex-1 px-3 py-2 min-w-0 gap-1">
                        {/* Row 1: filename + download + remove */}
                        <div className="flex items-center gap-2">
                            <VideoName video={video}/>

                            <VideoDownloadButton
                                jobId={video.jobId!}
                                fileName="output.mp4"
                                downloadName={`${video.name.replace(/\.[^.]+$/, '')}_${video.resolution}.mp4`}
                            />

                            <button
                                onClick={() => onRemove(processingType, video.id)}
                                className="shrink-0 flex items-center justify-center w-5 h-5 rounded text-stone-600 hover:text-stone-300 hover:bg-stone-700/40 transition-colors duration-100"
                            >
                                <X size={12} />
                            </button>
                        </div>

                        {/* Row 2: file size */}
                        <div className="flex items-center">
                            <span className="font-mono text-[10px] text-stone-500 tabular-nums">
                                {formatSize(video.size)}
                            </span>
                        </div>
                    </div>
                </li>
            ))}
        </ul>
    )
}

export { UploadVideoList, ProcessedVideoList }