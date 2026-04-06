import { useRef, useState } from "react"
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

    return { uploadedVideos, setUploadedVideos, removeUploadedVideo, startVideoUploads }
}