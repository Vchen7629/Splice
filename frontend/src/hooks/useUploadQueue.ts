import { useEffect, useRef, useState } from "react"
import { VideoService } from "../api/services/video"

export type JobStatus = 'pending' | 'uploading' | 'processing' | 'complete' | 'error'

export interface UploadedVideo {
    id: number
    name: string
    size: number
    resolution: string
    status: JobStatus
    uploadProgress: number
    jobId: string | null
    error?: string
}

export function useUploadQueue() {
    const [uploadedVideos, setUploadedVideos] = useState<UploadedVideo[]>([])
    const [processedVideos, setProcessedVideos] = useState<UploadedVideo[]>([])
    const abortRefs = useRef<Map<number, () => void>>(new Map())
    const completingIds = useRef<Set<number>>(new Set())
    const uploadedVideosRef = useRef<UploadedVideo[]>([])
    uploadedVideosRef.current = uploadedVideos

    function updateVideo(id: number, patch: Partial<UploadedVideo>) {
        setUploadedVideos(prev => prev.map(v => v.id === id ? { ...v, ...patch } : v))
    }

    function removeUploadedVideo(id: number) {
        abortRefs.current.get(id)?.()
        abortRefs.current.delete(id)
        setUploadedVideos(prev => prev.filter(v => v.id !== id))
    }

    function removeProcessedVideo(id: number) {
        setProcessedVideos(prev => prev.filter(v => v.id !== id))
    }

    function markComplete(video: UploadedVideo) {
        if (completingIds.current.has(video.id)) return
        completingIds.current.add(video.id)
        setUploadedVideos(prev => prev.filter(v => v.id !== video.id))
        setProcessedVideos(prev => [...prev, { ...video, status: 'complete' }])
    }

    async function pollStatus(video: UploadedVideo) {
        try {
            const data = await VideoService.status(video.jobId!)
            if (data.state === 'COMPLETE') markComplete(video)
            else if (data.state === 'ERROR') updateVideo(video.id, { status: 'error', error: data.error })
        } catch {
            updateVideo(video.id, { status: 'error' })
        }
    }

    function startVideoUploads(files: Map<number, File>) {
        setUploadedVideos(prev => prev.map(video => {
            if (video.status !== 'pending') return video

            const file = files.get(video.id)
            if (!file) return video

            const { promise, abort } = VideoService.upload(file, video.resolution, (pct) => {
                updateVideo(video.id, { uploadProgress: pct })
            })

            abortRefs.current.set(video.id, abort)

            promise
                .then(({ job_id }: { job_id: string }) => {
                    abortRefs.current.delete(video.id)
                    updateVideo(video.id, { jobId: job_id, status: 'processing', uploadProgress: 100 })
                })
                .catch((err: Error) => {
                    abortRefs.current.delete(video.id)
                    if (err.name !== 'AbortError') updateVideo(video.id, { status: 'error', error: err.message })
                })

            return { ...video, status: 'uploading' as JobStatus }
        }))
    }

    const processingCount = uploadedVideos.filter(v => v.status === 'processing' && v.jobId).length

    useEffect(() => {
        if (processingCount === 0) return

        const interval = setInterval(() => {
            const processing = uploadedVideosRef.current.filter(v => v.status === 'processing' && v.jobId)
            processing.forEach(pollStatus)
        }, 1000)

        return () => clearInterval(interval)
    }, [processingCount])

    return { uploadedVideos, setUploadedVideos, removeUploadedVideo, startVideoUploads, processedVideos, removeProcessedVideo }
}
