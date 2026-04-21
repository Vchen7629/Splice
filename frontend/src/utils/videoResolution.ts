export interface Resolution {
    label: string
    height: number
}

export const TRANSCODE_RESOLUTIONS: Resolution[] = [
    { label: '480p',  height: 480  },
    { label: '720p',  height: 720  },
    { label: '1080p', height: 1080 },
]

export const UPSCALE_RESOLUTIONS: Resolution[] = [
    { label: '720p',  height: 720  },
    { label: '1080p', height: 1080 },
    { label: '1440p', height: 1440 },
    { label: '4K',    height: 2160 },
]

export function defaultResolution(processingType: string, sourceHeight: number): string {
    if (processingType === 'Upscale') {
        return (
            UPSCALE_RESOLUTIONS.find((r: Resolution) => r.height > sourceHeight)?.label
            ?? UPSCALE_RESOLUTIONS[UPSCALE_RESOLUTIONS.length - 1].label
        )
    }
    return '1080p'
}

/**
 * helper function to detect the height of the uploaded video. Needed so upscaling
 * page can use this to only show resolutions that are bigger than the uploaded video
 * @param file - the video file to detect
 * @returns - an promise containing the video height number
 */
export function getVideoResolution(file: File): Promise<{ height: number}>{
    return new Promise((resolve, reject) => {
        const video = document.createElement('video')

        video.preload = 'metadata'
        video.onloadedmetadata = () => {
            resolve({ height: video.videoHeight })
            URL.revokeObjectURL(video.src)
        }

        video.onerror = reject

        video.src = URL.createObjectURL(file)
    })
}