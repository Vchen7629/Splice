export type JobStatus = 'pending' | 'uploading' | 'processing' | 'complete' | 'error' | 'degraded'

export type ProcessingType = 'Transcode' | 'Upscale' | 'Denoise' | 'Convert'

export interface UploadedVideo {
    id: number
    name: string
    size: number
    resolution: string
    sourceHeight: number
    status: JobStatus
    uploadProgress: number
    jobId: string | null
    stage?: string
    error?: string
}
