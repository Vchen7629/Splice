import type { ProcessedVideo } from "../types/file"

type OutputNameFields = Pick<ProcessedVideo, 'name' | 'processingType' | 'resolution'>

// convert the file size byte count to correct type
export function formatSize(bytes: number): string {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + ' GB'
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(0) + ' MB'
    return (bytes / 1e3).toFixed(0) + ' KB'
}

/**name the processed file is shown and saved as, e.g. clip_1080p.mp4 or clip.gif */
export function outputFileName(file: OutputNameFields): string {
    const baseName = file.name.replace(/\.[^.]+$/, '')
    const takesResolution = file.processingType === 'Transcode' || file.processingType === 'Upscale'
    const suffix = takesResolution ? `_${file.processingType.toLowerCase()}d_${file.resolution}` : ''
    
    return `${baseName}${suffix}.mp4`
}