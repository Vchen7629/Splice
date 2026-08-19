import { useEffect, useState } from 'react'

export type Mode = 'light' | 'dark'

const MODE_KEY = 'splice.mode'

/** The palette block in index.css that each mode selects. */
const PALETTE: Record<Mode, string> = {
    light: 'studio',
    dark:  'darkroom',
}

function readMode(): Mode {
    const stored = localStorage.getItem(MODE_KEY)
    if (stored === 'light' || stored === 'dark') return stored

    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
    const [mode, setMode] = useState<Mode>(() => {
        const initial = readMode()

        // Applied during render rather than in an effect: an effect runs after first
        // paint, which flashes the light default before a dark palette takes hold.
        document.documentElement.dataset.palette = PALETTE[initial]
        return initial
    })

    useEffect(() => {
        document.documentElement.dataset.palette = PALETTE[mode]
        localStorage.setItem(MODE_KEY, mode)
    }, [mode])

    return {
        mode,
        toggleMode: () => setMode(m => (m === 'light' ? 'dark' : 'light')),
    }
}
