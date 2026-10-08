import { useEffect, useRef } from "react";
import type { UploadedFile } from "../types/file";
import { useVideoQueueStore } from "../state/videoQueue";
import { VideoService } from "../api/services/video";
import { toast } from "sonner";

type ActiveJob = UploadedFile & { jobId: string }

const isActiveJob = (v: UploadedFile): v is ActiveJob => (
    v.status === 'processing' || 
    v.status === 'degraded' || 
    v.status === 'cancelling'
) && !!v.jobId

interface StatusEventData {
    job_id: string
    state: 'PROCESSING' | 'COMPLETE' | 'FAILED' | 'CANCELLED'
    stage: string
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

function openJobConnection(job: ActiveJob, connections: Map<string, EventSource>) {
    const es = VideoService.connectEvents(job.jobId)
    connections.set(job.jobId, es)

    es.addEventListener('status', (e: MessageEvent) => {
        const data: StatusEventData = JSON.parse(e.data)
        const { updateVideo, markComplete, markCancelled } = useVideoQueueStore.getState()
        
        switch (data.state) {
            case 'COMPLETE':
                es.close()
                connections.delete(job.jobId)
                markComplete(job.id)
                break
            case 'CANCELLED':
                es.close()
                connections.delete(job.jobId)
                markCancelled(job.id)
                break
            case 'FAILED':
                es.close()
                connections.delete(job.jobId)
                updateVideo(job.id, { status: 'error', error: data.error })
                toast.error(`${job.name} failed to ${job.processingType.toLowerCase()}`, { description: data.error })
                break
            case 'PROCESSING':
                updateVideo(job.id, { status: 'processing', stage: data.stage, jobProgress: undefined })
                break
        }
    })

    es.addEventListener('progress', (e: MessageEvent) => {
        const data: ProgressEventData = JSON.parse(e.data)
        const { videos, updateVideo } = useVideoQueueStore.getState()
        const current = videos.find(v => v.id === job.id)
        if (!current || current.jobId !== data.job_id || current.stage !==data.stage) return
        updateVideo(job.id, { jobProgress: data.progress })    
    })

    es.addEventListener('health', (e: MessageEvent) => {
        const data: HealthEventData = JSON.parse(e.data)
        const { updateVideo } = useVideoQueueStore.getState()

        if (data.state === 'DEGRADED') {
            updateVideo(job.id, { status: 'degraded', error: data.error })
        } else {
            updateVideo(job.id, { status: 'processing' })
        }
    })

    es.onerror = () => {
        // browsers auto-retry transient drops on their own; only react
        // once EventSource has fully given up (fatal, non-retryable).
        if (es.readyState == EventSource.CLOSED && connections.has(job.jobId)) {
            connections.delete(job.jobId)
            useVideoQueueStore.getState().updateVideo(job.id, { status: 'error' })
            toast.error(`${job.name} failed to ${job.processingType.toLowerCase()}`)
        }
    }
}

export function useJobEvents() {
    const connections = useRef(new Map<string, EventSource>())

    useEffect(() => {
        const activeConnections = connections.current

        function sync() {
            const current = useVideoQueueStore.getState().videos.filter(isActiveJob)
            const currentIds = new Set(current.map(j => j.jobId))

            for (const [jobId, es] of activeConnections) {
                if (!currentIds.has(jobId)) {
                    es.close()
                    activeConnections.delete(jobId)
                }
            }

            for (const job of current) {
                if (activeConnections.has(job.jobId)) continue

                openJobConnection(job, activeConnections)
            }
        }

        sync()
        const unsubscribe = useVideoQueueStore.subscribe(sync)

        return () => {
            unsubscribe()
            activeConnections.forEach(es => es.close())
            activeConnections.clear()
        }
    }, [])
}