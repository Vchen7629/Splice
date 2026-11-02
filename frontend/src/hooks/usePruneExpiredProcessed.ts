import { useEffect } from "react"
import { useProcessedStore } from "../state/processedVideos"

const PRUNE_INTERVAL_MS = 30 * 1000

export function usePruneExpiredProcessed() {
    useEffect(() => {
        const prune = useProcessedStore.getState().prune
        prune()

        const id = setInterval(prune, PRUNE_INTERVAL_MS)
        const onVisible = () => { 
            if (document.visibilityState === 'visible') prune()
        }

        return () => {
            clearInterval(id)
            document.removeEventListener('visibilitychange', onVisible)
        }
    }, [])
}