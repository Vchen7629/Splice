export type JobStatus = 'pending' | 'uploading' | 'processing' | 'complete' | 'error' | 'degraded' | 'cancelling' | 'cancelled'
export type ProcessingType = 'Transcode' | 'Upscale' | 'Denoise' | 'Convert'

export const DENOISE_QUALITIES = ['light', 'medium'] as const
export const TARGET_FORMATS = ["mp4", "mov", "mkv", "webm", "gif"] as const
export type DenoiseQuality = typeof DENOISE_QUALITIES[number]
export type TargetFormat = typeof TARGET_FORMATS[number]

interface baseFile {
    name: string
    resolution: string
    processingType: ProcessingType
    targetFormat?: TargetFormat
}

export interface UploadedFile extends baseFile {
    id: number
    size: number
    sourceHeight: number
    status: JobStatus
    uploadProgress: number
    jobId: string | null
    denoiseQuality?: DenoiseQuality
    stage?: string
    jobProgress?: number
    error?: string
}

export interface ProcessedVideo extends baseFile {
    jobId: string
    completedAt: number
}