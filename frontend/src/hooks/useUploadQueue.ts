import { useEffect, useRef, useState } from "react"
import { VideoService } from "../api/services/video"

export type JobStatus = 'pending' | 'uploading'| 'processing' | 'complete' | 'error'

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
    const abortRefs = useRef<Map<number, () => void>>(new Map())

    function updateUploadedVideo(id: number, patch: Partial<UploadedVideo>) {
        setUploadedVideos(prev => prev.map(j => j.id === id ? { ...j, ...patch } : j))
    }

    // Removes a uploaded video from the queue and aborts its upload if in progress
    function removeUploadedVideo(id: number) {
        abortRefs.current.get(id)?.()
        abortRefs.current.delete(id)
        setUploadedVideos(prev => prev.filter(j => j.id !== id))
    }

    function startVideoUploads(files: Map<number, File>) {
        setUploadedVideos(prev => prev.map(video => {
            if (video.status !== 'pending') return video

            const file = files.get(video.id)
            if (!file) return video

            const { promise, abort } = VideoService.upload(file, video.resolution, (pct) => {
                updateUploadedVideo(video.id, { uploadProgress: pct })
            })

            abortRefs.current.set(video.id, abort)

            promise
                .then(({ job_id }: { job_id: string }) => {
                    abortRefs.current.delete(video.id)
                    updateUploadedVideo(video.id, { jobId: job_id, status: 'processing', uploadProgress: 100 })
                })
                .catch((err: Error) => {
                    abortRefs.current.delete(video.id)
                    if (err.name !== 'AbortError') {
                        updateUploadedVideo(video.id, { status: 'error', error: err.message })
                    }
                })
            
            return { ...video, status: 'uploading' as JobStatus}
        }))
    }

    const processingCount = uploadedVideos.filter(v => v.status === 'processing' && v.jobId).length

    useEffect(() => {
        if (processingCount === 0) return

        const interval = setInterval(async () => {
            setUploadedVideos(prev => {
                const processing = prev.filter(v => v.status === 'processing' && v.jobId)
                processing.forEach(async (video) => {
                    try {
                        const data = await VideoService.status(video.jobId!)
                        if (data.state === 'COMPLETE') {
                            updateUploadedVideo(video.id, { status: 'complete' })
                        } else if (data.state === 'ERROR') {
                            updateUploadedVideo(video.id, { status: 'error', error: data.error })
                        }
                    } catch {
                        updateUploadedVideo(video.id, { status: 'error' })
                    }
                })
                return prev
            })
        }, 1000)

        return () => clearInterval(interval)
    }, [processingCount])

    return { uploadedVideos, setUploadedVideos, removeUploadedVideo, startVideoUploads }
}