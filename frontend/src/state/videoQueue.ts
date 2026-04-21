import { create } from "zustand"
import type { ProcessingType, UploadedVideo } from "../types/video"

interface VideoQueueStore {
    uploadedVideos: Record<ProcessingType, UploadedVideo[]>
    processedVideos: Record<ProcessingType, UploadedVideo[]>
    addVideos: (processingType: ProcessingType, videos: UploadedVideo[]) => void
    updateVideoStatus: (processingType: ProcessingType, id: number, patch: Partial<UploadedVideo>) => void
    setResolution: (processingType: ProcessingType, id: number, resolution: string) => void
    removeUploadedVideo: (processingType: ProcessingType, id: number) => void
    removeProcessedVideo: (processingType: ProcessingType, id: number) => void
    markComplete: (processingType: ProcessingType, video: UploadedVideo) => void
    resetVideo: (processingType: ProcessingType, id: number) => void
}

const completingIds = new Set<number>()

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
            uploadedVideos: {
                ...state.uploadedVideos, 
                [processingType]: [...state.uploadedVideos[processingType], ...videos] 
            }
        })),

    updateVideoStatus: (processingType, id, patch) => 
        set(state => ({
            uploadedVideos: {
                ...state.uploadedVideos,
                [processingType]: state.uploadedVideos[processingType].map(v => v.id === id ? { ...v, ...patch } : v)
            }
        })),
    
    setResolution: (processingType, id, resolution) =>
        set(state => ({
            uploadedVideos: {
                ...state.uploadedVideos,
                [processingType]: state.uploadedVideos[processingType].map(v => v.id === id ? { ...v, resolution} : v)
            }
        })),
    
    removeUploadedVideo: (processingType, id) =>
        set(state => ({ 
            uploadedVideos: {
                ...state.uploadedVideos,
                [processingType]: state.uploadedVideos[processingType].filter(v => v.id !== id) 
            }
        })),

    removeProcessedVideo: (processingType, id) =>
        set(state => ({ 
            processedVideos: {
                ...state.uploadedVideos,
                [processingType]: state.processedVideos[processingType].filter(v => v.id !== id) 
            }
        })),

    resetVideo: (processingType, id) =>
        set(state => ({
            uploadedVideos: {
                ...state.uploadedVideos, 
                [processingType]: state.uploadedVideos[processingType].map(v =>
                    v.id === id ? { ...v, status: 'pending', error: undefined, uploadProgress: 0 } : v
                )
            }
        })),

    markComplete: (processingType, video) => {
        if (completingIds.has(video.id)) return

        completingIds.add(video.id)
        set(state => ({
            uploadedVideos: {
                ...state.uploadedVideos,
                [processingType]: state.uploadedVideos[processingType].filter(v => v.id !== video.id),
            },
            processedVideos: {
                ...state.processedVideos, 
                [processingType]: [...state.processedVideos[processingType], { ...video, status: 'complete' }]
            },
        }))
    },
}))