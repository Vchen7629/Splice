import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { ProcessedVideo } from "../types/file"

const STORAGE_KEY = "splice.processed"

interface ProcessedStore {
    processed: ProcessedVideo[]
    add: (video: ProcessedVideo) => void
    remove: (jobId: string) => void
}

export const useProcessedStore = create<ProcessedStore>()(
    persist(
        (set) => ({
            processed: [],

            add: (video) => 
                set(state => 
                    state.processed.some(v => v.jobId === video.jobId)
                        ? state
                        : { processed: [...state.processed, video]}
                ),

            remove: (jobId) => set(state => ({ processed: state.processed.filter(v => v.jobId !== jobId )})),
        }),
        {
            name: STORAGE_KEY,
            version: 1,
            merge: (persisted, current) => {
                const raw = (persisted as Partial<ProcessedStore> | undefined)?.processed
                const saved = Array.isArray(raw)
                    ? raw.filter(v => v && typeof v.jobId === "string" && typeof v.processingType === "string")
                    : []
                return { ...current, processed: saved} /**TODO: in the future when we implement ttl on backend storage ttl, filter for ttl*/
            }
        }
    )
)

/**This is for keeping 2 window tabs in sync */
window.addEventListener("storage", (e) => {
    if (e.key === STORAGE_KEY) void useProcessedStore.persist.rehydrate()
})