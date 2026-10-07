import { create } from "zustand"
import type { UploadedFile } from "../types/file"
import { useProcessedStore } from "./processedVideos"

interface VideoQueueStore {
    videos: UploadedFile[]
    cancelled: UploadedFile[]
    addVideos: (videos: UploadedFile[]) => void
    updateVideo: (id: number, patch: Partial<UploadedFile>) => void
    setResolution: (id: number, resolution: string) => void
    removeVideo: (id: number) => void
    removeCancelled: (id: number) => void
    markComplete: (id: number) => void
    markCancelling: (id: number) => void
    markCancelled: (id: number) => void
    resetVideo: (id: number) => void
}

export const useVideoQueueStore = create<VideoQueueStore>((set, get) => ({
    videos: [],
    cancelled: [],

    addVideos: (videos) => set(state => ({ videos: [...state.videos, ...videos ]})),

    updateVideo: (id, patch) =>
        set(state => ({ 
            videos: state.videos.map(v => v.id === id ? { ...v, ...patch} : v)
        })),

    setResolution: (id, resolution) => get().updateVideo(id, { resolution }),

    removeVideo: (id) => set(state => ({ videos: state.videos.filter(v => v.id !== id) })),

    removeCancelled: (id) => set(state => ({ cancelled: state.cancelled.filter(v => v.id !== id) })),

    markComplete: (id) => {
        const video = get().videos.find(v => v.id === id)
        if (!video?.jobId) return

        useProcessedStore.getState().add({
            jobId: video.jobId,
            name: video.name,
            resolution: video.resolution,
            processingType: video.processingType,
            completedAt: Date.now(),
        })
        set(state => ({ videos: state.videos.filter(v => v.id !== id)}))
    },

    markCancelling: (id) => get().updateVideo(id, { status: 'cancelling' }),

    markCancelled: (id) => {
        const video = get().videos.find(v => v.id === id)
        if (!video) return

        set(state => ({
            videos: state.videos.filter(v => v.id !== id),
            cancelled: [...state.cancelled, { ...video, status: 'cancelled' }],
        }))
    },

    resetVideo: (id) => get().updateVideo(id, { status: 'pending', error: undefined, uploadProgress: 0 }),
}))
