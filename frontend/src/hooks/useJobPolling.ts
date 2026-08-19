import { useEffect } from "react";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import type { ProcessingType, UploadedFile } from "../types/file";

async function pollVideo(file: UploadedFile, processingType: ProcessingType) {                                                                                                                                   
    const { updateVideoStatus, markComplete } = useVideoQueueStore.getState()
    try {
        const data = await VideoService.status(file.jobId!)
        if (data.state === 'COMPLETE') markComplete(processingType, file)
        else if (data.state === 'FAILED') updateVideoStatus(processingType, file.id, { status: 'error', error: data.error })
        else if (data.state === 'DEGRADED') updateVideoStatus(processingType, file.id, { status: 'degraded', stage: data.stage, error: data.error })
        else if (data.state === 'PROCESSING') updateVideoStatus(processingType, file.id, { status: 'processing', stage: data.stage, jobProgress: data.progress ?? undefined })
    } catch {
        updateVideoStatus(processingType, file.id, { status: 'error' })
    }
}

const isActivePoll = (v: UploadedFile) => (v.status === 'processing' || v.status === 'degraded') && !!v.jobId

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