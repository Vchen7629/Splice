import { Download } from "lucide-react"
import { VideoService } from "../api/services/video"

interface VideoDownloadButtonProps {
    jobId: string
    fileName: string
    downloadName: string
}

const VideoDownloadButton = ({ jobId, fileName, downloadName }: VideoDownloadButtonProps) => {
    async function handleDownload() {
        const blob = await VideoService.download(jobId, fileName)
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')

        a.href = url
        a.download = downloadName
        a.click()

        URL.revokeObjectURL(url)
    }
    return (
        <button
            onClick={handleDownload}
            className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none"
        >
            <Download size={16} className="text-green-400 transition-colors duration-0.1s hover:text-accent"/>
        </button>
    )
}

export default VideoDownloadButton