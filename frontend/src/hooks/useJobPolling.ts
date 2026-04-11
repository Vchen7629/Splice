import { useEffect } from "react";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import type { UploadedVideo } from "../types/video";

async function pollVideo(video: UploadedVideo) {                                                                                                                                   
    const { updateVideo, markComplete } = useVideoQueueStore.getState()
    try {
        const data = await VideoService.status(video.jobId!)
        if (data.state === 'COMPLETE') markComplete(video)
        else if (data.state === 'ERROR') updateVideo(video.id, { status: 'error', error: data.error })
    } catch {
        updateVideo(video.id, { status: 'error' })
    }
}

export function useJobPolling() {
    const processingCount = useVideoQueueStore(
        s => s.uploadedVideos.filter(v => v.status === 'processing' && v.jobId).length
    )

    useEffect(() => {
        if (processingCount === 0) return

        const interval = setInterval(() => {
            const { uploadedVideos } = useVideoQueueStore.getState()

            uploadedVideos
                .filter(v => v.status === 'processing' && v.jobId)
                .forEach(pollVideo)
        }, 1000)

        return () => clearInterval(interval)
    }, [processingCount])
}