import { CheckCheck, Loader, Video, X } from "lucide-react"
import type { JobStatus, UploadedVideo } from "../types/video"
import { formatSize, truncateName } from "../utils/fileDisplay"
import { useVideoQueueStore } from "../state/videoQueue"

interface UploadVideoListProps {
    videos: UploadedVideo[]
    onRemove: (id: number) => void
}

const RESOLUTIONS = ['480p', '720p', '1080p']

const UploadVideoList = ({ videos, onRemove }: UploadVideoListProps) => {
    const { uploadedVideos, resetVideo, setResolution } = useVideoQueueStore()

    function StatusIcon({ status }: { status: JobStatus }) {
        if (status === 'processing') return <Loader size={18} className="text-accent shrink-0 animate-spin" />
        if (status === 'complete') return <CheckCheck size={18} className="text-green-400 shrink-0" />
        if (status === 'error') return <X size={18} className="text-red-400 shrink-0"/>
        return <Video size={18} className="text-accent shrink-0" />
    }

    function handleSetResolution(id: number, resolution: string) {
        const video = uploadedVideos.find(v => v.id === id)

        if (video?.status === "error") {
            resetVideo(id)
        }
        
        setResolution(id, resolution)
    }

    return (
        <ul className="flex flex-col gap-2 p-3 flex-1 overflow-y-auto bg-row-bg">
            {videos.map(video => (
                <li
                    key={video.id}
                    className={`flex flex-col px-3 py-2 shrink-0 rounded-lg border-1 border-x-2
                        bg-panel ${video.status === "error" ? "border-x-red-500/60 border-y-red-900/40" : "border-x-accent border-y-cyan-800/40"}
                        transition-[transform,background] duration-200 hover:[transform:scaleX(1.01)] hover:bg-[#2a2a2e]`}
                >
                <div className="flex items-center gap-3 h-[40px]">
                    <StatusIcon status={video.status} />

                    <span className="flex-1 text-xs truncate font-mono" title={video.name}>
                        {truncateName(video.name)}
                    </span>

                    <span className="text-xs shrink-0 tabular-nums font-mono text-[11px] text-right min-w-[48px] text-zinc-400">
                        {formatSize(video.size)}
                    </span>

                    <select
                        value={video.resolution}
                        onChange={e => handleSetResolution(video.id, e.target.value)}
                        className="resolution-select text-xs text-[11px] font-mono rounded-md outline-none shrink-0 w-[72px] h-[28px] pl-[8px] bg-input-bg border-1 border-zinc-700"
                    >
                        {RESOLUTIONS.map(r => <option key={r} value={r}>{r}</option>)}
                    </select>

                    <button
                        onClick={() => onRemove(video.id)}
                        className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none"
                    >
                        <X size={16} className='text-zinc-400 transition-colors duration-0.1s hover:text-accent'/>
                    </button>
                </div>

                {video.status === 'uploading' && (
                    <div className="h-1 w-full rounded-full bg-zinc-700 mt-1">
                        <div
                            className="h-1 rounded-full bg-accent transition-all duration-200"
                            style={{ width: `${video.uploadProgress}%` }}
                        />
                    </div>
                )}
                </li>
            ))}
        </ul>
    )
}

export default UploadVideoList