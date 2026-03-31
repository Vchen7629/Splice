import { Video, X } from 'lucide-react'

interface StubItem {
  id: string
  name: string
  size: string
  resolution: string
}

const STUB_FILES: StubItem[] = [
  { id: '1', name: 'video-final.mp4',      size: '412 MB', resolution: '720p'  },
  { id: '2', name: 'raw-cut-scene-02.mov', size: '1.2 GB', resolution: '1080p' },
  { id: '3', name: 'clip-01.mkv',          size: '88 MB',  resolution: '480p'  },
]

const VideoUploadQueue = () => {
    const count = STUB_FILES.length

    // Truncate long filenames while keeping the extension visible
    function truncateName(name: string): string {
        const dotIdx = name.lastIndexOf('.')
        const ext  = dotIdx !== -1 ? name.slice(dotIdx) : ''
        const base = dotIdx !== -1 ? name.slice(0, dotIdx) : name
        if (base.length <= 18) return name

        return base.slice(0, 14) + '…' + ext
    }

    return (
        <section className="flex flex-col w-[45%] rounded-xl overflow-hidden bg-panel border-1 border-line">
        <div className="flex items-center justify-between px-4 shrink-0 h-[48px] border-b-1 border-line">
            <span className="text-xs font-medium tracking-widest uppercase text-text-1 font-mono tracking-[0.12em]">
                Queue
            </span>
            <span className="text-xs tabular-nums font-mono text-accent tracking-widest uppercase">
                {count} {count === 1 ? 'file' : 'files'}
            </span>
        </div>
        <ul className="flex flex-col gap-2 p-3 flex-1 overflow-y-auto bg-row-bg">
            {STUB_FILES.map((item, _i) => (
            <li
                key={item.id}
                className="flex items-center gap-3 px-3 shrink-0 h-[56px] rounded-lg bg-panel border-1 border-line border-x-2 border-x-accent transition-[transform,box-shadow,background] duration-150 hover:[transform:scaleX(1.015)] hover:bg-[#2a2a2e] hover:shadow-[0_0_12px_var(--accent-glow),rgba(0,0,0,0.35)_0_2px_8px_-2px]"
            >
                <Video className='text-accent shrink-0' size={18}/>

                <span className="flex-1 text-xs truncate font-mono" title={item.name}>
                    {truncateName(item.name)}
                </span>

                <span className="text-xs shrink-0 tabular-nums font-mono text-[11px] text-right min-w-[48px] text-zinc-400">
                    {item.size}
                </span>

                <select
                    defaultValue={item.resolution}
                    className="resolution-select text-xs text-[11px] font-mono rounded-md outline-none shrink-0 w-[72px] h-[28px] pl-[8px] bg-input-bg border-1 border-zinc-700"
                >
                    <option value="480p">480p</option>
                    <option value="720p">720p</option>
                    <option value="1080p">1080p</option>
                </select>

                <button className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none">
                    <X size={16} className='text-zinc-400 transition-colors duration-0.1s hover:text-accent'/>
                </button>
            </li>
            ))}
        </ul>

        <div className="p-4 shrink-0 border-t-1 border-line">
            <button className="transcode-btn w-full rounded-lg text-sm font-medium h-[40px] text-white bg-accent-btn">
                Transcode {count} {count === 1 ? 'file' : 'files'} →
            </button>
        </div>
        </section>
    )
}

export default VideoUploadQueue
