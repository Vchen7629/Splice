
// Truncate long filenames while keeping the extension visible
export function truncateName(name: string): string {
    const dotIdx = name.lastIndexOf('.')
    const ext  = dotIdx !== -1 ? name.slice(dotIdx) : ''
    const base = dotIdx !== -1 ? name.slice(0, dotIdx) : name
    if (base.length <= 18) return name

    return base.slice(0, 14) + '…' + ext
}

// convert the file size byte count to correct type
export function formatSize(bytes: number): string {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + ' GB'
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(0) + ' MB'
    return (bytes / 1e3).toFixed(0) + ' KB'
}