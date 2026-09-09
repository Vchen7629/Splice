import { useRef } from "react"
import { toast } from "sonner"
import { VideoService } from "../api/services/video"
import { useVideoQueueStore } from "../state/videoQueue"
import type { ProcessingType, UploadedFile } from "../types/file"

export function useUploadQueue(processingType: ProcessingType) {
    const abortRefs = useRef<Map<number, () => void>>(new Map())
    const removeUploadedVideoFromStore = useVideoQueueStore(s => s.removeUploadedVideo)
    const markCancelling = useVideoQueueStore(s => s.markCancelling)
    const updateVideoStatus = useVideoQueueStore(s => s.updateVideoStatus)

    function removeUploadedVideo(processingType: ProcessingType, id: number) {
        abortRefs.current.get(id)?.()
        abortRefs.current.delete(id)
        removeUploadedVideoFromStore(processingType, id)
    }

    function cancelVideo(processingType: ProcessingType, file: UploadedFile) {
        if (!file.jobId) return
        const previousStatus = file.status

        markCancelling(processingType, file.id)

        VideoService.cancel(file.jobId).catch((err: Error) => {
            updateVideoStatus(processingType, file.id, { status: previousStatus })
            toast.error(`Failed to cancel ${file.name}`, { description: err.message })
        })
    }

    function startVideoUploads(files: Map<number, File>) {
        const { uploadedVideos } = useVideoQueueStore.getState()

        uploadedVideos[processingType].forEach(video => {
            if (video.status !== 'pending') return video

            const file = files.get(video.id)
            if (!file) return video

            const sourceResolution = video.sourceHeight > 0 ? `${video.sourceHeight}p` : ""

            const { promise, abort } = VideoService.upload(file, video.resolution, sourceResolution, processingType, (pct) => {
                updateVideoStatus(processingType, video.id, { uploadProgress: pct })
            })

            abortRefs.current.set(video.id, abort)
            updateVideoStatus(processingType, video.id, { status: 'uploading' })

            promise
                .then(({ job_id }: { job_id: string }) => {
                    abortRefs.current.delete(video.id)
                    updateVideoStatus(processingType, video.id, { jobId: job_id, status: 'processing', uploadProgress: 100 })
                })
                .catch((err: Error) => {
                    abortRefs.current.delete(video.id)
                    if (err.name !== 'AbortError') {
                        updateVideoStatus(processingType, video.id, { status: 'error', error: err.message })
                        toast.error(`${file.name} failed to upload`, { description: err.message })
                    }
                })
        })
    }

    return { removeUploadedVideo, cancelVideo, startVideoUploads }
}
