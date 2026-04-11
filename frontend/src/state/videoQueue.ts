import { create } from "zustand"
import type { UploadedVideo } from "../types/video"

interface VideoQueueStore {
    uploadedVideos: UploadedVideo[]
    processedVideos: UploadedVideo[]
    addVideos: (videos: UploadedVideo[]) => void
    updateVideo: (id: number, patch: Partial<UploadedVideo>) => void
    setResolution: (id: number, resolution: string) => void
    removeUploadedVideo: (id: number) => void
    removeProcessedVideo: (id: number) => void
    markComplete: (video: UploadedVideo) => void
    resetVideo: (id: number) => void
}

const completingIds = new Set<number>()

export const useVideoQueueStore = create<VideoQueueStore>((set) => ({
    uploadedVideos: [],
    processedVideos: [],

    addVideos: (videos) => 
        set(state => ({ uploadedVideos: [...state.uploadedVideos, ...videos ]})),

    updateVideo: (id, patch) => 
        set(state => ({
            uploadedVideos: state.uploadedVideos.map(v => v.id === id ? { ...v, ...patch } : v)
        })),
    
    setResolution: (id, resolution) =>
        set(state => ({
            uploadedVideos: state.uploadedVideos.map(v => v.id === id ? { ...v, resolution} : v)
        })),
    
    removeUploadedVideo: (id) =>
        set(state => ({ uploadedVideos: state.uploadedVideos.filter(v => v.id !== id) })),

    removeProcessedVideo: (id) =>
        set(state => ({ processedVideos: state.processedVideos.filter(v => v.id !== id) })),

    resetVideo: (id) =>
        set(state => ({
            uploadedVideos: state.uploadedVideos.map(v =>
                v.id === id ? { ...v, status: 'pending', error: undefined, uploadProgress: 0 } : v
            )
        })),

    markComplete: (video) => {
        if (completingIds.has(video.id)) return

        completingIds.add(video.id)
        set(state => ({
            uploadedVideos: state.uploadedVideos.filter(v => v.id !== video.id),
            processedVideos: [...state.processedVideos, { ...video, status: 'complete' }],
        }))
    },
}))