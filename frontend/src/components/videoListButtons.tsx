import { toast } from "sonner"
import type { ProcessingType, UploadedVideo } from "../types/video"
import { useEffect, type RefObject } from "react"
import { useUploadQueue } from "../hooks/useUploadQueue"
import { VideoService } from "../api/services/video"
import { Download, X } from "lucide-react"

interface VideoUploadButtonProps {
    videos: UploadedVideo[]
    fileMap: RefObject<Map<number, File>>
    processingType: ProcessingType
}

/**
 * Appears on the bottom of the processing queue component, uploads all added videos to
 * the backend using onStartUploads
 */
const VideoUploadButton = ({ videos, fileMap, processingType }: VideoUploadButtonProps) => {
    const { startVideoUploads } = useUploadQueue(processingType)

    const pendingCount = videos.filter(v => v.status === 'pending').length
    const erroredVideo = videos.find(v => v.status === 'error')

    useEffect(() => {
        if (!erroredVideo) return
        toast.error(`Failed to upload "${erroredVideo.name}": ${erroredVideo.error ?? 'Unknown error'}`)
    }, [erroredVideo?.id, erroredVideo?.status])

    return (
        <div className="p-4 shrink-0 border-t border-[var(--border)]">
            <button
                onClick={() => startVideoUploads(fileMap.current)}
                disabled={pendingCount === 0}
                className="transcode-btn w-full rounded-[10px] text-sm font-medium h-[40px] text-white bg-amber-600 border border-amber-700 hover:bg-amber-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors duration-150"
            >
                {processingType} {pendingCount} {pendingCount === 1 ? 'file' : 'files'} →
            </button>
        </div>
    )
}


interface VideoDownloadButtonProps {
    jobId: string
    fileName: string
    downloadName: string
}

/**
 * button for the user to download the file to their machine. Only shown on the Output panel
 * @param jobId - the jobId to use to fetch video from the backend
 * @param fileName - the fileName to use to fetch video from the backend
 * @param downloadName - the name of the file saved to the users machine 
 */
const VideoDownloadButton = ({ jobId, fileName, downloadName }: VideoDownloadButtonProps) => {
    async function handleDownload() {
        const blob = await VideoService.download(jobId, fileName)
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')

        a.href = url
        a.download = downloadName
        a.click()

        URL.revokeObjectURL(url)
    }
    
    return (
        <button
            onClick={handleDownload}
            className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none"
        >
            <Download size={16} className="text-green-400 transition-colors duration-0.1s hover:text-accent"/>
        </button>
    )
}

interface VideoRemoveButtonProps {
    video: UploadedVideo
    processingType: ProcessingType
    onRemove: (processingType: ProcessingType, id: number) => void
}

/**
 * Button for the user to remove the video from the processed/upload video list
 * @param video - the video to remove
 * @param 
 * @returns 
 */
const VideoRemoveButton = ({ video, processingType, onRemove }: VideoRemoveButtonProps) => {

    return (
        <button
            onClick={() => onRemove(processingType, video.id)}
            className="shrink-0 flex items-center justify-center w-5 h-5 rounded text-stone-600 hover:text-stone-300 hover:bg-stone-700/40 transition-colors duration-100"
        >
            <X size={12} />
        </button>
    )
}

export { VideoUploadButton, VideoDownloadButton, VideoRemoveButton }
