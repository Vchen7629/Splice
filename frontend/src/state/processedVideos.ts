import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { ProcessedVideo } from "../types/file"

const STORAGE_KEY = "splice.processed"
const PROCESSED_TTL_MS = 3 *24 * 60 * 60 * 1000 // keep in sync with backend processed video storage TTL (3 days)
const EXPIRY_BUFFER_MS = 30 * 1000 // hide slightly early (30s early) so we never show a link that just 404s

const isExpired = (v: ProcessedVideo, now: number) => 
    now - v.completedAt >= PROCESSED_TTL_MS - EXPIRY_BUFFER_MS

interface ProcessedStore {
    processed: ProcessedVideo[]
    add: (video: ProcessedVideo) => void
    remove: (jobId: string) => void
    prune: () => void
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
            prune: () => set(state => {
                const now = Date.now()
                const notExpired = state.processed.filter(v => !isExpired(v, now))

                return notExpired.length === state.processed.length ? state : { processed: notExpired }
            })
        }),
        {
            name: STORAGE_KEY,
            version: 1,
            merge: (persisted, current) => {
                const raw = (persisted as Partial<ProcessedStore> | undefined)?.processed
                const now = Date.now()
                const saved = Array.isArray(raw)
                    ? raw.filter(v => v && 
                        typeof v.jobId === "string" && 
                        typeof v.processingType === "string" &&
                        typeof v.completedAt === "number" &&
                        !isExpired(v, now)
                    )
                    : []
                return { ...current, processed: saved}
            }
        }
    )
)

/**This is for keeping 2 window tabs in sync */
window.addEventListener("storage", (e) => {
    if (e.key === STORAGE_KEY) void useProcessedStore.persist.rehydrate()
})