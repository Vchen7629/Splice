import { useRef, useState, type ChangeEvent, type DragEvent } from 'react'
import { CollectFilesFromDrop, FilterFiles } from '../utils/fileUpload'

/**
 * Drag & drop plus click-to-browse wiring, shared by every layout's drop surface.
 * Only the presentation differs between layouts — the file handling does not.
 */
export function useFileDrop(onFiles: (files: File[]) => void) {
    const [isDragging, setIsDragging] = useState(false)
    const inputRef = useRef<HTMLInputElement>(null)

    function handleDragOver(e: DragEvent<HTMLElement>) {
        e.preventDefault()
        setIsDragging(true)
    }

    function handleDragLeave(e: DragEvent<HTMLElement>) {
        if (e.currentTarget.contains(e.relatedTarget as Node | null)) return
        setIsDragging(false)
    }

    async function handleDrop(e: DragEvent<HTMLElement>) {
        e.preventDefault()
        setIsDragging(false)

        const { accepted } = FilterFiles(await CollectFilesFromDrop(e))
        if (accepted.length > 0) onFiles(accepted)
    }

    function handleInputChange(e: ChangeEvent<HTMLInputElement>) {
        const { accepted } = FilterFiles(Array.from(e.target.files ?? []))
        if (accepted.length > 0) onFiles(accepted)

        e.target.value = ''
    }

    return {
        isDragging,
        inputRef,
        browse: () => inputRef.current?.click(),
        dropHandlers: { onDragOver: handleDragOver, onDragLeave: handleDragLeave, onDrop: handleDrop },
        handleInputChange,
    }
}
