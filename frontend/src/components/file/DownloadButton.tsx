import { Download } from 'lucide-react'
import { VideoService } from '../../api/services/video'
import { useState } from 'react';
import { toast } from 'sonner';

const DownloadButton = ({ file }: { file: { jobId: string | null; name: string; resolution: string } }) => {
    const [isDownloading, setIsDownloading] = useState(false)

    async function handleDownload() {
        setIsDownloading(true)
        let url: string | undefined

        try {
            const blob = await VideoService.download(file.jobId!, 'output.mp4')
            url = URL.createObjectURL(blob)
            const a = document.createElement('a')

            a.href = url
            a.download = `${file.name.replace(/\.[^.]+$/, '')}_${file.resolution}.mp4`
            a.click()
        } catch {
            toast.error(`Failed to download ${file.name}`)
        } finally {
            if (url) URL.revokeObjectURL(url)
            setIsDownloading(false)
        }
    }

    return (
        <button
            onClick={handleDownload}
            disabled={isDownloading}
            aria-label={`Download ${file.name}`}
            className="shrink-0 flex items-center justify-center w-6 h-6 rounded text-status-done hover:text-fg-strong transition-colors duration-100 disabled:opacity-50"
        >
            <Download size={15} />
        </button>
    )
}

export default DownloadButton