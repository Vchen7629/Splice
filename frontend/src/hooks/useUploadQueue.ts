import { toast } from "sonner"
import { VideoService } from "../api/services/video"
import { useVideoQueueStore } from "../state/videoQueue"
import type { ProcessingType, UploadedFile } from "../types/file"
import { abortRefs, fileMap } from "../state/fileRegistry"

export function useUploadQueue() {
    const removeVideo = useVideoQueueStore(s => s.removeVideo)
    const markCancelling = useVideoQueueStore(s => s.markCancelling)
    const updateVideo = useVideoQueueStore(s => s.updateVideo)

    function removeUploadedVideo(id: number) {
        abortRefs.get(id)?.()
        abortRefs.delete(id)
        fileMap.delete(id)
        removeVideo(id)
    }

    function cancelVideo(file: UploadedFile) {
        if (!file.jobId) return
        const previousStatus = file.status

        markCancelling(file.id)

        VideoService.cancel(file.jobId).catch((err: Error) => {
            updateVideo(file.id, { status: previousStatus })
            toast.error(`Failed to cancel ${file.name}`, { description: err.message })
        })
    }

    function startVideoUploads(processingType: ProcessingType) {
        useVideoQueueStore.getState().videos
            .filter(v => v.processingType === processingType && v.status === 'pending')
            .forEach(video => {
                const file = fileMap.get(video.id)
                if (!file) return video

                const sourceResolution = video.sourceHeight > 0 ? `${video.sourceHeight}p` : ""

                const { promise, abort } = VideoService.upload(file, video.resolution, sourceResolution, processingType, (pct) => {
                    updateVideo(video.id, { uploadProgress: pct })
                })

                abortRefs.set(video.id, abort)
                updateVideo(video.id, { status: 'uploading' })

                promise
                    .then(({ job_id }: { job_id: string }) => {
                        abortRefs.delete(video.id)
                        updateVideo(video.id, { jobId: job_id, status: 'processing', uploadProgress: 100 })
                    })
                    .catch((err: Error) => {
                        abortRefs.delete(video.id)
                        if (err.name !== 'AbortError') {
                            updateVideo(video.id, { status: 'error', error: err.message })
                            toast.error(`${file.name} failed to upload`, { description: err.message })
                        }
                    })
        })
    }

    return { removeUploadedVideo, cancelVideo, startVideoUploads }
}
