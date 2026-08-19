import { Download } from 'lucide-react'
import { VideoService } from '../../api/services/video'

const DownloadButton = ({ file }: { file: { jobId: string | null; name: string; resolution: string } }) => {
    async function handleDownload() {
        const blob = await VideoService.download(file.jobId!, 'output.mp4')
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')

        a.href = url
        a.download = `${file.name.replace(/\.[^.]+$/, '')}_${file.resolution}.mp4`
        a.click()

        URL.revokeObjectURL(url)
    }

    return (
        <button
            onClick={handleDownload}
            aria-label={`Download ${file.name}`}
            className="shrink-0 flex items-center justify-center w-6 h-6 rounded text-status-done hover:text-fg-strong transition-colors duration-100"
        >
            <Download size={15} />
        </button>
    )
}

export default DownloadButton