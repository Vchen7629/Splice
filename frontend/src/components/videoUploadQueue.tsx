import { CheckCheck, Loader, Video, X } from 'lucide-react'
import type { JobStatus, UploadedVideo } from '../types/video'
import { formatSize, truncateName } from '../utils/fileDisplay'

const RESOLUTIONS = ['480p', '720p', '1080p']

interface VideoUploadQueueProps {
    videos: UploadedVideo[]
    onRemove: (id: number) => void
    onSetResolution: (id: number, resolution: string) => void
    onStartUploads: () => void
}

const VideoUploadQueue = ({ videos, onRemove, onSetResolution, onStartUploads}: VideoUploadQueueProps) => {
    const count = videos.length
    const pendingCount = videos.filter(v => v.status === 'pending').length

    function StatusIcon({ status }: { status: JobStatus }) {
        if (status === 'processing') return <Loader size={18} className="text-accent shrink-0 animate-spin" />
        if (status === 'complete') return <CheckCheck size={18} className="text-green-400 shrink-0" />
        return <Video size={18} className="text-accent shrink-0" />
    }

    return (
        <section className="flex flex-col w-full h-[60%] bg-panel border-1 border-line rounded-xl overflow-hidden">
            <div className="flex items-center justify-between px-4 shrink-0 h-[48px] border-b-1 border-line">
                <span className="text-xs font-medium tracking-widest uppercase text-text-1 font-mono tracking-[0.12em]">
                    Queue
                </span>
                <span className="text-xs tabular-nums font-mono text-accent tracking-widest uppercase">
                    {count} {count === 1 ? 'file' : 'files'}
                </span>
            </div>
            <ul className="flex flex-col gap-2 p-3 flex-1 overflow-y-auto bg-row-bg">
                {videos.map(video => (
                    <li
                        key={video.id}
                        className="flex flex-col px-3 py-2 shrink-0 rounded-lg bg-panel border-1 border-line border-x-2 border-x-accent transition-[transform,box-shadow,background] duration-150 hover:[transform:scaleX(1.015)] hover:bg-[#2a2a2e] hover:shadow-[0_0_12px_var(--accent-glow),rgba(0,0,0,0.35)_0_2px_8px_-2px]"
                    >
                    <div className="flex items-center gap-3 h-[40px]">
                        <StatusIcon status={video.status} />

                        <span className="flex-1 text-xs truncate font-mono" title={video.name}>
                            {truncateName(video.name)}
                        </span>

                        <span className="text-xs shrink-0 tabular-nums font-mono text-[11px] text-right min-w-[48px] text-zinc-400">
                            {formatSize(video.size)}
                        </span>

                        <select
                            value={video.resolution}
                            disabled={video.status !== 'pending'}
                            onChange={e => onSetResolution(video.id, e.target.value)}
                            className="resolution-select text-xs text-[11px] font-mono rounded-md outline-none shrink-0 w-[72px] h-[28px] pl-[8px] bg-input-bg border-1 border-zinc-700"
                        >
                            {RESOLUTIONS.map(r => <option key={r} value={r}>{r}</option>)}
                        </select>

                        <button
                            onClick={() => onRemove(video.id)}
                            className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none"
                        >
                            <X size={16} className='text-zinc-400 transition-colors duration-0.1s hover:text-accent'/>
                        </button>
                    </div>

                    {video.status === 'uploading' && (
                        <div className="h-1 w-full rounded-full bg-zinc-700 mt-1">
                            <div
                                className="h-1 rounded-full bg-accent transition-all duration-200"
                                style={{ width: `${video.uploadProgress}%` }}
                            />
                        </div>
                    )}
                    </li>
                ))}
            </ul>

            <div className="p-4 shrink-0 border-t-1 border-line">
                <button
                    onClick={onStartUploads}
                    disabled={pendingCount === 0}
                    className="transcode-btn w-full rounded-lg text-sm font-medium h-[40px] text-white bg-accent-btn disabled:opacity-40 disabled:cursor-not-allowed"
                >
                    Transcode {pendingCount} {pendingCount === 1 ? 'file' : 'files'} →
                </button>
            </div>
        </section>
    )
}

export default VideoUploadQueue
