import { X } from "lucide-react";
import type { ProcessingType, UploadedFile } from "../../types/file";
import { STATUS_BG } from "./StatusStyles";
import { formatSize } from "../../utils/fileDisplay";
import ResolutionSelect from "./ResolutionSelect";
import PipelineLane from "./PipelineProgress";
import DownloadButton from "./DownloadButton";

interface UploadedFilesQueueListProps {
    queue: UploadedFile[]
    activeFeature: ProcessingType
    showResolution: boolean
    onRemove: (id: number) => void
    handleSetResolution: (id: number, resolution: string) => void
}

const UploadedFileQueueList = (
    { queue, activeFeature, showResolution, onRemove, handleSetResolution }: UploadedFilesQueueListProps
) => {
    if (queue.length === 0) {
        return (
            <div className="flex flex-col gap-1.5 px-5 py-6">
                <span className="text-caption text-fg">Nothing queued</span>
                <span className="text-meta text-fg-muted leading-relaxed">
                    Files you add appear here with the stage they are in.
                </span>
            </div>
        )
    }

    return (
        <ul className="flex flex-col flex-1 overflow-y-auto">
            {queue.map(file => (
                <li
                    key={file.id}
                    className="relative flex flex-col gap-2.5 px-5 py-3.5 border-b border-line hover:bg-row transition-colors duration-150"
                >
                    <span className={`absolute left-0 top-3.5 bottom-3.5 w-[2px] rounded-full ${STATUS_BG[file.status]}`}/>
                    
                    <div className="flex items-baseline gap-2">
                        <span className="flex-1 min-w-0 truncate font-mono text-meta text-fg-strong" title={file.name}>
                            {file.name}
                        </span>
                        <button
                            onClick={() => onRemove(file.id)}
                            aria-label={`Remove ${file.name}`}
                            className="shrink-0 flex items-center justify-center w-5 h-5 rounded text-fg-faint hover:text-fg-strong transition-colors duration-100"
                        >
                            <X size={12}/>
                        </button>
                    </div>

                    <div className="flex items-center gap-2.5"> 
                        <span className="font-mono text-eyebrow text-fg-faint tabular-nums">
                            {formatSize(file.size)}
                        </span>
                        {showResolution && (
                            <span className="ml-auto">
                                <ResolutionSelect 
                                    processingType={activeFeature}
                                    file={file}
                                    handleSetResolution={handleSetResolution}
                                />
                            </span>
                        )}
                    </div>
                    
                    {/*NOTE: maybe rename this to be clearer */}
                    <PipelineLane file={file} processingType={activeFeature}/>
                </li>
            ))}         
        </ul>
    )
}

interface ProcessedVideosListProps {
    processedVideos: UploadedFile[]
    onRemove: (id: number) => void
}

const ProcessedVideosList = ({ processedVideos, onRemove }: ProcessedVideosListProps) => {
    if (processedVideos.length === 0) {
        return (
            <div className="px-5 py-4">
                <span className="text-meta text-fg-muted leading-relaxed">
                    Finished files land here, ready to download
                </span>
            </div>
        )
    }

    return (
        <ul className="flex flex-col overflow-y-auto">
            {processedVideos.map(file => (
                <li
                    key={file.id}
                    className="relative flex items-center gap-2.5 px-5 py-3 border-b border-line hover:bg-row transition-colors duration-150"
                >
                    <span className="absolute left-0 top-3 bottom-3 w-[2px] rounded-full bg-status-done"/>
                    <span className="flex flex-col gap-0.5 flex-1 min-w-0">
                        <span className="truncate font-mono text-meta text-fg-strong" title={file.name}>
                            {file.name}
                        </span>
                        <span className="font-mono text-eyebrow text-fg-faint tabular-nums">
                            {formatSize(file.size)} · {file.resolution}
                        </span>
                    </span>
                    <DownloadButton file={file}/>
                    <button
                        onClick={() => onRemove(file.id)}
                        aria-label={`Remove ${file.name}`}
                        className="shrink-0 flex items-center justify-center w-5 h-5 rounded text-fg-faint hover:text-fg-strong transition-colors duration-100"
                    >
                        <X size={12}/>
                    </button>
                </li>
            ))}
        </ul>
    )
}

interface CancelledVideosProps {
    cancelledVideos: UploadedFile[]
    onRemove: (id: number) => void
}

const CancelledVideosList = ({ cancelledVideos, onRemove }: CancelledVideosProps) => {
    if (cancelledVideos.length === 0) {
        return (
            <div className="px-5 py-4">
                <span className="text-meta text-fg-muted leading-relaxed">
                    Videos Cancelled while processing land here
                </span>
            </div>
        )
    }

    return (
        <ul className="flex flex-col overflow-y-auto">
            {cancelledVideos.map(file => (
                <li
                    key={file.id}
                    className="relative flex items-center gap-2.5 px-5 py-3 border-b border-line hover:bg-row transition-colors duration-150"
                >
                    <span className="absolute left-0 top-3 bottom-3 w-[2px] rounded-full bg-blue-400"/>
                    <span className="flex flex-col gap-0.5 flex-1 min-w-0 font-mono text-meta text-fg-strong">
                        {file.name}
                    </span>
                    <button
                        onClick={() => onRemove(file.id)}
                        aria-label={`Remove ${file.name}`}
                        className="shrink-0 flex items-center justify-center w-5 h-5 rounded text-fg-faint hover:text-fg-strong transition-colors duration-100"
                    >
                        <X size={12}/>
                    </button>
                </li>
            ))}
        </ul>
    )
}

export { UploadedFileQueueList, ProcessedVideosList, CancelledVideosList }