export type JobStatus = 'pending' | 'uploading' | 'processing' | 'complete' | 'error' | 'degraded' | 'cancelling' | 'cancelled'

export type ProcessingType = 'Transcode' | 'Upscale' | 'Denoise' | 'Convert'

interface baseFile {
    name: string
    resolution: string
    processingType: ProcessingType
}

export interface UploadedFile extends baseFile {
    id: number
    size: number
    sourceHeight: number
    status: JobStatus
    uploadProgress: number
    jobId: string | null
    stage?: string
    jobProgress?: number
    error?: string
}

export interface ProcessedVideo extends baseFile {
    jobId: string
    completedAt: number
}