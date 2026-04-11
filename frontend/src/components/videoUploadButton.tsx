import { toast } from "sonner"
import type { UploadedVideo } from "../types/video"
import { useEffect } from "react"

interface VideoUploadButtonProps {
    videos: UploadedVideo[]
    onStartUploads: () => void
}

const VideoUploadButton = ({ videos, onStartUploads }: VideoUploadButtonProps) => {
    const pendingCount = videos.filter(v => v.status === 'pending').length
    const erroredVideo = videos.find(v => v.status === 'error')

    useEffect(() => {                                                                                                                                                                  
        if (!erroredVideo) return                                                                                                                                                      
        toast.error(`Failed to upload "${erroredVideo.name}": ${erroredVideo.error ?? 'Unknown error'}`)                                                                               
    }, [erroredVideo?.id, erroredVideo?.status])  

    return (
        <div className="p-4 shrink-0 border-t-1 border-line">
            <button
                onClick={onStartUploads}
                disabled={pendingCount === 0}
                className="transcode-btn w-full rounded-lg text-sm font-medium h-[40px] text-white bg-accent-btn disabled:opacity-40 disabled:cursor-not-allowed"
            >
                Transcode {pendingCount} {pendingCount === 1 ? 'file' : 'files'} →
            </button>
        </div>
    )
}

export default VideoUploadButton