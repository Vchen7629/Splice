import { toast } from "sonner"
import type { UploadedVideo } from "../types/video"
import { useEffect, type RefObject } from "react"
import { useUploadQueue } from "../hooks/useUploadQueue"

interface VideoUploadButtonProps {
    videos: UploadedVideo[]
    fileMap: RefObject<Map<number, File>>
}

/**
 * Appears on the bottom of the processing queue component, uploads all added videos to
 * the backend using onStartUploads
 * @param videos - list of unprocessed videos we are uploading
 * @param fileMap - ref mapping each video's numeric ID to its original File object, 
 * used to retrieve the actual file data when uploading
 */
const VideoUploadButton = ({ videos, fileMap }: VideoUploadButtonProps) => {
    const { startVideoUploads } = useUploadQueue()

    const pendingCount = videos.filter(v => v.status === 'pending').length
    const erroredVideo = videos.find(v => v.status === 'error')

    useEffect(() => {                                                                                                                                                                  
        if (!erroredVideo) return                                                                                                                                                      
        toast.error(`Failed to upload "${erroredVideo.name}": ${erroredVideo.error ?? 'Unknown error'}`)                                                                               
    }, [erroredVideo?.id, erroredVideo?.status])  

    return (
        <div className="p-4 shrink-0 border-t-1 border-line">
            <button
                onClick={() => startVideoUploads(fileMap.current)}
                disabled={pendingCount === 0}
                className="transcode-btn w-full rounded-lg text-sm font-medium h-[40px] text-white bg-accent-btn disabled:opacity-40 disabled:cursor-not-allowed"
            >
                Transcode {pendingCount} {pendingCount === 1 ? 'file' : 'files'} →
            </button>
        </div>
    )
}

export default VideoUploadButton