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
            <span className="text-xs px-2 py-0.5 rounded-full tabular-nums bg-accent-bg border-1 border-accent-border font-mono text-accent">
                {count}
            </span>
        </div>
        <ul className="flex-1 overflow-y-auto" style={{ background: 'var(--bg-row)' }}>
            {STUB_FILES.map((item, _i) => (
            <li
                key={item.id}
                className="flex items-center gap-3 px-4 border-b-1 border-line h-[56px]"
            >
                <Video className='text-accent' size={20}/>

                <span className="flex-1 text-xs truncate font-mono" title={item.name}>
                    {truncateName(item.name)}
                </span>

                <span className="text-xs shrink-0 tabular-nums font-mono text-[11px] text-right min-w-[48px]">
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

                <button className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-base leading-none transition-colors duration-0.1s hover:text-accent">
                    <X size={20}/>
                </button>
            </li>
            ))}
        </ul>

        <div className="p-4 shrink-0 border-t-1 border-line">
            <button
            className="transcode-btn w-full rounded-lg text-sm font-medium h-[40px] text-white bg-accent-btn"
            style={{
                boxShadow: '0 0 18px var(--accent-btn-glow), 0 4px 12px rgba(0,0,0,0.35)',
                letterSpacing: '0.01em',
                transition: 'box-shadow 0.15s, opacity 0.15s',
            }}
            >
            Transcode {count} {count === 1 ? 'file' : 'files'} →
            </button>
        </div>
        </section>
    )
}

export default VideoUploadQueue
