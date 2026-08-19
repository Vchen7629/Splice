import { create } from "zustand"
import type { ProcessingType, UploadedFile } from "../types/file"

interface VideoQueueStore {
    uploadedVideos: Record<ProcessingType, UploadedFile[]>
    processedVideos: Record<ProcessingType, UploadedFile[]>
    addVideos: (processingType: ProcessingType, videos: UploadedFile[]) => void
    updateVideoStatus: (processingType: ProcessingType, id: number, patch: Partial<UploadedFile>) => void
    setResolution: (processingType: ProcessingType, id: number, resolution: string) => void
    removeUploadedVideo: (processingType: ProcessingType, id: number) => void
    removeProcessedVideo: (processingType: ProcessingType, id: number) => void
    markComplete: (processingType: ProcessingType, video: UploadedFile) => void
    resetVideo: (processingType: ProcessingType, id: number) => void
}

/** Replaces one processingType's list within a queues record, leaving the others untouched. */
function withUpdatedQueue(
    queues: Record<ProcessingType, UploadedFile[]>,
    processingType: ProcessingType,
    updater: (list: UploadedFile[]) => UploadedFile[]
): Record<ProcessingType, UploadedFile[]> {
    return { ...queues, [processingType]: updater(queues[processingType]) }
}

export const useVideoQueueStore = create<VideoQueueStore>((set) => ({
    uploadedVideos: {
        "Transcode": [],
        "Upscale": [],
        "Denoise": [],
        "Convert": []
    },
    processedVideos: {
        "Transcode": [],
        "Upscale": [],
        "Denoise": [],
        "Convert": []
    },

    addVideos: (processingType, videos) =>
        set(state => ({
            uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list => [...list, ...videos])
        })),

    updateVideoStatus: (processingType, id, patch) =>
        set(state => ({
            uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list =>
                list.map(v => v.id === id ? { ...v, ...patch } : v)
            )
        })),

    setResolution: (processingType, id, resolution) =>
        set(state => ({
            uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list =>
                list.map(v => v.id === id ? { ...v, resolution } : v)
            )
        })),

    removeUploadedVideo: (processingType, id) =>
        set(state => ({
            uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list =>
                list.filter(v => v.id !== id)
            )
        })),

    removeProcessedVideo: (processingType, id) =>
        set(state => ({
            processedVideos: withUpdatedQueue(state.processedVideos, processingType, list =>
                list.filter(v => v.id !== id)
            )
        })),

    resetVideo: (processingType, id) =>
        set(state => ({
            uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list =>
                list.map(v => v.id === id ? { ...v, status: 'pending', error: undefined, uploadProgress: 0 } : v)
            )
        })),

    markComplete: (processingType, video) => 
        set(state => {
            const current = state.uploadedVideos[processingType]
                .find(v => v.id === video.id)
            if (!current) return {}

            return {
                uploadedVideos: withUpdatedQueue(state.uploadedVideos, processingType, list =>
                    list.filter(v => v.id !== video.id)
                ),
                processedVideos: withUpdatedQueue(state.processedVideos, processingType, list =>
                    [...list, { ...current, status: 'complete' }]
                ),
            }
        }),
}))
