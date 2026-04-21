import { useEffect } from "react";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import type { ProcessingType, UploadedVideo } from "../types/video";

async function pollVideo(video: UploadedVideo, processingType: ProcessingType) {                                                                                                                                   
    const { updateVideoStatus, markComplete } = useVideoQueueStore.getState()
    try {
        const data = await VideoService.status(video.jobId!)
        if (data.state === 'COMPLETE') markComplete(processingType, video)
        else if (data.state === 'FAILED') updateVideoStatus(processingType, video.id, { status: 'error', error: data.error })
        else if (data.state === 'DEGRADED') updateVideoStatus(processingType, video.id, { status: 'degraded', stage: data.stage, error: data.error })
        else if (data.state === 'PROCESSING') updateVideoStatus(processingType, video.id, { status: 'processing', stage: data.stage })
    } catch {
        updateVideoStatus(processingType, video.id, { status: 'error' })
    }
}

const isActivePoll = (v: UploadedVideo) => (v.status === 'processing' || v.status === 'degraded') && !!v.jobId

export function useJobPolling() {
    const activeCount = useVideoQueueStore(
        s => Object.values(s.uploadedVideos).flat().filter(isActivePoll).length
    )

    useEffect(() => {
        if (activeCount === 0) return

        const interval = setInterval(() => {
            const { uploadedVideos } = useVideoQueueStore.getState()

            Object.entries(uploadedVideos).forEach(([feature, videos]) => {
                videos.filter(isActivePoll).forEach(video => pollVideo(video, feature as ProcessingType))
            })
        }, 1000)

        return () => clearInterval(interval)
    }, [activeCount])
}