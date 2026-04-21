import { useEffect, useState, type RefObject } from 'react'
import type { ProcessingType, UploadedVideo } from '../types/video'
import { UploadVideoList, ProcessedVideoList } from './videoList'
import { VideoUploadButton } from './videoListButtons'

type PanelTab = 'queue' | 'output'

interface VideoPanelProps {
    uploadedVideos: Record<ProcessingType, UploadedVideo[]>
    processedVideos: Record<ProcessingType, UploadedVideo[]>
    onRemove: (processingType: ProcessingType, id: number) => void
    onRemoveProcessed: (processingType: ProcessingType, id: number) => void
    fileMap: RefObject<Map<number, File>>
    processingType: ProcessingType
}

const VideoPanel = ({ uploadedVideos, processedVideos, onRemove, onRemoveProcessed, fileMap, processingType }: VideoPanelProps) => {
    const [activeTab, setActiveTab] = useState<PanelTab>('queue')

    // Auto-switch to Output when a new video completes
    useEffect(() => {
        if (processedVideos[processingType].length > 0) {
            setActiveTab('output')
        }
    }, [processedVideos[processingType].length])

    const showResolution = processingType !== 'Denoise'

    return (
        <div className="flex flex-col flex-1 h-[65vh] aspect-square bg-panel border border-[var(--border)] rounded-[10px] overflow-hidden">
            {/* Tab bar */}
            <div className="flex items-center gap-1 px-3 h-[44px] shrink-0 border-b border-[var(--border)]">
                <TabButton
                    label="Queue"
                    count={uploadedVideos[processingType].length}
                    active={activeTab === 'queue'}
                    onClick={() => setActiveTab('queue')}
                />
                <TabButton
                    label="Output"
                    count={processedVideos[processingType].length}
                    active={activeTab === 'output'}
                    onClick={() => setActiveTab('output')}
                />
            </div>

            {/* Content */}
            {activeTab === 'queue' ? (
                <>
                    <UploadVideoList processingType={processingType} videos={uploadedVideos[processingType]} onRemove={onRemove} showResolution={showResolution} />
                    <VideoUploadButton videos={uploadedVideos[processingType]} fileMap={fileMap} processingType={processingType}/>
                </>
            ) : (
                <ProcessedVideoList processingType={processingType} processedVideos={processedVideos[processingType]} onRemove={onRemoveProcessed} />
            )}
        </div>
    )
}

interface TabButtonProps {
    label: string
    count: number
    active: boolean
    onClick: () => void
}

const TabButton = ({ label, count, active, onClick }: TabButtonProps) => (
    <button
        onClick={onClick}
        className={`
            relative flex items-center gap-1.5 px-3 h-full text-[11px] font-medium tracking-wide transition-colors duration-100
            ${active ? 'text-amber-400' : 'text-stone-500 hover:text-stone-300'}
        `}
    >
        {label}:
        {count > 0 && (
            <span className={`text-[10px] font-mono tabular-nums ${active ? 'text-amber-500' : 'text-stone-600'}`}>
             {count}
            </span>
        )}
        {active && (
            <span className="absolute bottom-0 left-0 right-0 h-[2px] bg-amber-400 rounded-t-full" />
        )}
    </button>
)

export default VideoPanel
