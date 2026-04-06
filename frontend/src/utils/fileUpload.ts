import type { DragEvent } from "react"

export const ALLOWED_EXTENSIONS = ['.mp4', '.mov', '.mkv', '.webm', '.avi']
const MAX_FILE_SIZE_BYTES = 10 * 1024 * 1024 * 1024 // 10 GB


interface filterResult {
    accepted: File[]
    rejected: fileRejection[]
}

interface fileRejection {
    file: File
    error: string
}

/**
 * Extracts all files from a drag-and-drop event, including files inside dropped folders.
 * Uses the FileSystem API (webkitGetAsEntry) to recursively traverse directories.
 * Falls back to the plain file list if the FileSystem API is unavailable.
 * @param event - the browser DragEvent from an onDrop handler
 * @returns a flat array of all File objects found
 */
export async function CollectFilesFromDrop(event: DragEvent): Promise<File[]> {
    const items = Array.from(event.dataTransfer?.items ?? [])

    const entries = items
        .map((item) => item.webkitGetAsEntry())
        .filter((e): e is FileSystemEntry => e !== null)

    if (entries.length > 0) {
        const nested = await Promise.all(entries.map(collectFromEntry))
        return nested.flat()
    }

    // fallback to plain files
    return Array.from(event.dataTransfer?.files ?? [])
}

/**
 * Splits a list of files into accepted and rejected buckets based on extension and size rules.
 * @param files - array of files to filter
 * @returns object with accepted File[] and rejected array of { file, error } pairs
 */
export function FilterFiles(files: File[]): filterResult {
    const accepted: File[] = []
    const rejected: fileRejection[] = []

    for (const file of files) {
        const error = validateFile(file)
        error ? rejected.push({ file, error}) : accepted.push(file)
    }

    return { accepted, rejected }
}

/**
 * Recursively collects File objects from a FileSystemEntry.
 * If the entry is a file, wraps the callback-based API in a promise and returns it.
 * If the entry is a directory, reads its contents and recurses into each child entry.
 * @param entry - a FileSystemEntry (either FileSystemFileEntry or FileSystemDirectoryEntry)
 * @returns a flat array of all File objects under this entry
 */
async function collectFromEntry(entry: FileSystemEntry): Promise<File[]> {
    if (entry.isFile) {
        return new Promise<File[]>((resolve, reject) =>
            (entry as FileSystemFileEntry).file(
                (f) => resolve([f]), reject
            )
        )
    }

    if (entry.isDirectory) {
        const reader = (entry as FileSystemDirectoryEntry).createReader()
        const entries = await new Promise<FileSystemEntry[]>((resolve, reject) =>
            reader.readEntries(resolve, reject)
        )
        const nested = await Promise.all(entries.map(collectFromEntry))

        return nested.flat()
    }

    return []
}

/**
 * Validates a single file against allowed extensions and the size limit.
 * @param file - the video file to validate
 * @returns an error string describing the problem, or null if the file is valid
 */
function validateFile(file: File): string | null {
    const ext = '.' + file.name.split('.').pop()?.toLowerCase()
    if (!ALLOWED_EXTENSIONS.includes(ext)) {
        return `Unsupported format: ${ext}`
    }
    if (file.size > MAX_FILE_SIZE_BYTES) {
        return `File exceeds 10 GB limit`
    }
    return null
}
