import { useRef } from "react"
import { VideoService } from "../api/services/video"
import { useVideoQueueStore } from "../state/videoQueue"
import type { ProcessingType } from "../types/video"

export function useUploadQueue(processingType: ProcessingType) {
    const abortRefs = useRef<Map<number, () => void>>(new Map())
    const removeUploadedVideoFromStore = useVideoQueueStore(s => s.removeUploadedVideo)

    function removeUploadedVideo(processingType: ProcessingType, id: number) {
        abortRefs.current.get(id)?.()
        abortRefs.current.delete(id)
        removeUploadedVideoFromStore(processingType, id)
    }

    function startVideoUploads(files: Map<number, File>) {
        const { uploadedVideos, updateVideoStatus } = useVideoQueueStore.getState()

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
                    }
                })
        })
    }

    return { removeUploadedVideo, startVideoUploads }
}
