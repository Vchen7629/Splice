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
