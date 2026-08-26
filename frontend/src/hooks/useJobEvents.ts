import { useEffect, useRef } from "react";
import type { ProcessingType, UploadedFile } from "../types/file";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import { toast } from "sonner";

const isActiveJob = (v: UploadedFile) => (v.status === 'processing' || v.status === 'degraded') && !!v.jobId

interface ActiveJob {
    jobId: string
    file: UploadedFile
    processingType: ProcessingType
}

interface StatusEventData {
    job_id: string
    state: 'PROCESSING' | 'COMPLETE' | 'FAILED'
    stage: string
    progress?: number
    error?: string
}

interface ProgressEventData {
    job_id: string
    stage: string
    progress: number
}

interface HealthEventData {
    state: 'PROCESSING' | 'DEGRADED'
    error?: string
}

function activeJobs(uploadedVideos: Record<ProcessingType, UploadedFile[]>): ActiveJob[] {
    return Object.entries(uploadedVideos).flatMap(([processingType, videos]) =>
        videos.filter(isActiveJob).map(v => ({ jobId: v.jobId!, file: v, processingType: processingType as ProcessingType}))
    )
}

function openJobConnection(job: ActiveJob, connections: Map<string, EventSource>) {
    const es = VideoService.connectEvents(job.jobId)
    connections.set(job.jobId, es)

    es.addEventListener('status', (e: MessageEvent) => {
        const data: StatusEventData = JSON.parse(e.data)
        const { updateVideoStatus, markComplete } = useVideoQueueStore.getState()
        
        switch (data.state) {
            case 'COMPLETE':
                es.close()
                connections.delete(job.jobId)
                markComplete(job.processingType, job.file)
                break
            case 'FAILED':
                es.close()
                connections.delete(job.jobId)
                updateVideoStatus(job.processingType, job.file.id, { status: 'error', error: data.error })
                toast.error(`${job.file.name} failed to ${job.processingType.toLowerCase()}`, { description: data.error })
                break
            case 'PROCESSING':
                updateVideoStatus(job.processingType, job.file.id, { status: 'processing', stage: data.stage, jobProgress: data.progress ?? undefined })
                break
        }
    })

    es.addEventListener('progress', (e: MessageEvent) => {
        const data: ProgressEventData = JSON.parse(e.data)
        const store = useVideoQueueStore.getState()
        const current = store.uploadedVideos[job.processingType]
            .find(video => video.id === job.file.id)
        
        if (!current || current.jobId !== data.job_id || current.stage !==data.stage) return
        
        useVideoQueueStore.getState().updateVideoStatus(job.processingType, job.file.id, { jobProgress: data.progress })
    })

    es.addEventListener('health', (e: MessageEvent) => {
        const data: HealthEventData = JSON.parse(e.data)
        const { updateVideoStatus } = useVideoQueueStore.getState()

        if (data.state === 'DEGRADED') {
            updateVideoStatus(job.processingType, job.file.id, { status: 'degraded', error: data.error })
        } else {
            updateVideoStatus(job.processingType, job.file.id, { status: 'processing' })
        }
    })

    es.onerror = () => {
        // browsers auto-retry transient drops on their own; only react
        // once EventSource has fully given up (fatal, non-retryable)
        if (es.readyState == EventSource.CLOSED) {
            connections.delete(job.jobId)
            useVideoQueueStore.getState().updateVideoStatus(job.processingType, job.file.id, { status: 'error' })
            toast.error(`${job.file.name} failed to ${job.processingType.toLowerCase()}`)
        }
    }
}

export function useJobEvents() {
    const connections = useRef(new Map<string, EventSource>())

    useEffect(() => {
        function sync() {
            const current = activeJobs(useVideoQueueStore.getState().uploadedVideos)
            const currentIds = new Set(current.map(j => j.jobId))

            for (const [jobId, es] of connections.current) {
                if (!currentIds.has(jobId)) {
                    es.close()
                    connections.current.delete(jobId)
                }
            }

            for (const job of current) {
                if (connections.current.has(job.jobId)) continue
                
                openJobConnection(job, connections.current)
            }
        }

        sync()
        const unsubscribe = useVideoQueueStore.subscribe(sync)

        return () => {
            unsubscribe()
            connections.current.forEach(es => es.close())
            connections.current.clear()
        }
    }, [])
}