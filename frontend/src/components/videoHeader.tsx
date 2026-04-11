import type { UploadedVideo } from "../types/video"

/**
 * Header component for the video component
 * @param param0 
 * @returns 
 */
const VideoHeader = ({ videos, title }: { videos: UploadedVideo[], title: string }) => {
    const count = videos.length

    return (
        <div className="flex items-center justify-between px-4 shrink-0 h-[48px] border-b-1 border-line">
            <span className="text-xs font-medium tracking-widest uppercase text-text-1 font-mono tracking-[0.12em]">
                {title}
            </span>
            <span className="text-xs tabular-nums font-mono text-accent tracking-widest uppercase">
                {count} {count === 1 ? 'file' : 'files'}
            </span>
        </div>
    )
}

export default VideoHeader