import { useEffect } from "react";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import type { UploadedVideo } from "../types/video";

async function pollVideo(video: UploadedVideo) {                                                                                                                                   
    const { updateVideo, markComplete } = useVideoQueueStore.getState()
    try {
        const data = await VideoService.status(video.jobId!)
        if (data.state === 'COMPLETE') markComplete(video)
        else if (data.state === 'FAILED') updateVideo(video.id, { status: 'error', error: data.error })
        else if (data.state === 'DEGRADED') updateVideo(video.id, { status: 'degraded', stage: data.stage, error: data.error })
        else if (data.state === 'PROCESSING') updateVideo(video.id, { status: 'processing', stage: data.stage })
    } catch {
        updateVideo(video.id, { status: 'error' })
    }
}

const isActivePoll = (v: UploadedVideo) => (v.status === 'processing' || v.status === 'degraded') && !!v.jobId

export function useJobPolling() {
    const activeCount = useVideoQueueStore(
        s => s.uploadedVideos.filter(isActivePoll).length
    )

    useEffect(() => {
        if (activeCount === 0) return

        const interval = setInterval(() => {
            const { uploadedVideos } = useVideoQueueStore.getState()

            uploadedVideos
                .filter(isActivePoll)
                .forEach(pollVideo)
        }, 1000)

        return () => clearInterval(interval)
    }, [activeCount])
}