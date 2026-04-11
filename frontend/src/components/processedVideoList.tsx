import { CheckCheck, X } from "lucide-react"
import type { UploadedVideo } from "../types/video"
import { formatSize, truncateName } from "../utils/fileDisplay"
import VideoDownloadButton from "./videoDownloadButton"


const ProcessedVideoList = ({ processedVideos, onRemove}: { processedVideos: UploadedVideo[], onRemove: (id: number) => void}) => {
    return (
        <ul className="flex flex-col gap-2 p-3 flex-1 overflow-y-auto bg-row-bg">
            {processedVideos.map(video => (
                <li
                    key={video.id}
                    className="flex flex-col px-3 py-2 shrink-0 rounded-lg bg-panel border-1 border-line border-x-2 border-x-accent transition-[transform,box-shadow,background] duration-150 hover:[transform:scaleX(1.015)] hover:bg-[#2a2a2e] hover:shadow-[0_0_12px_var(--accent-glow),rgba(0,0,0,0.35)_0_2px_8px_-2px]"
                >
                <div className="flex items-center gap-3 h-[40px]">
                    <CheckCheck size={18} className="text-green-400 shrink-0" />

                    <span className="flex-1 text-xs truncate font-mono" title={video.name}>
                        {truncateName(video.name)}
                    </span>

                    <span className="text-xs shrink-0 tabular-nums font-mono text-[11px] text-right min-w-[48px] text-zinc-400">
                        {formatSize(video.size)}
                    </span>

                    <VideoDownloadButton
                        jobId={video.jobId!}
                        fileName="output.mp4"
                        downloadName={`${video.name.replace(/\.[^.]+$/, '')}_${video.resolution}.mp4`}
                    />

                    <button
                        onClick={() => onRemove(video.id)}
                        className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none"
                    >
                        <X size={16} className='text-zinc-400 transition-colors duration-0.1s hover:text-accent'/>
                    </button>
                </div>
                </li>
            ))}
        </ul>
    )
}

export default ProcessedVideoList