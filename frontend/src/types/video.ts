export type JobStatus = 'pending' | 'uploading' | 'processing' | 'complete' | 'error' | 'degraded'

export interface UploadedVideo {
    id: number
    name: string
    size: number
    resolution: string
    status: JobStatus
    uploadProgress: number
    jobId: string | null
    stage?: string
    error?: string
}
