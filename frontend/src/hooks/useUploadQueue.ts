import { useRef } from "react"
import { VideoService } from "../api/services/video"
import { useVideoQueueStore } from "../state/videoQueue"

export function useUploadQueue() {
    const abortRefs = useRef<Map<number, () => void>>(new Map())
    const removeUploadedVideoFromStore = useVideoQueueStore(s => s.removeUploadedVideo)

    function removeUploadedVideo(id: number) {
        abortRefs.current.get(id)?.()
        abortRefs.current.delete(id)
        removeUploadedVideoFromStore(id)
    }

    function startVideoUploads(files: Map<number, File>) {
        const { uploadedVideos, updateVideo } = useVideoQueueStore.getState()

        uploadedVideos.forEach(video => {
            if (video.status !== 'pending') return video

            const file = files.get(video.id)
            if (!file) return video

            const { promise, abort } = VideoService.upload(file, video.resolution, (pct) => {
                updateVideo(video.id, { uploadProgress: pct })
            })

            abortRefs.current.set(video.id, abort)
            updateVideo(video.id, { status: 'uploading' })

            promise
                .then(({ job_id }: { job_id: string }) => {
                    abortRefs.current.delete(video.id)
                    updateVideo(video.id, { jobId: job_id, status: 'processing', uploadProgress: 100 })
                })
                .catch((err: Error) => {
                    abortRefs.current.delete(video.id)
                    if (err.name !== 'AbortError') updateVideo(video.id, { status: 'error', error: err.message })
                })
        })
    }

    return { removeUploadedVideo, startVideoUploads }
}
