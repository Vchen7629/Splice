import { X } from "lucide-react";
import { DENOISE_QUALITIES, TARGET_FORMATS, type DenoiseQuality, type ProcessedVideo, type ProcessingType, type TargetFormat, type UploadedFile } from "../../types/file";
import { STATUS_BG } from "./StatusStyles";
import { formatSize, outputFileName } from "../../utils/fileDisplay";
import PipelineLane from "./PipelineProgress";
import DownloadButton from "./DownloadButton";
import OptionDropdown from "../ui/OptionDropdown";
import { resolutionOptions } from "../../utils/videoResolution";

interface UploadedFilesQueueListProps {
    queue: UploadedFile[]
    activeFeature: ProcessingType
    onRemove: (id: number) => void
    handleSetResolution: (id: number, resolution: string) => void
    handleSetDenoiseQuality: (id: number, quality: DenoiseQuality) => void
    handleSetTargetFormat: (id: number, targetFormat: TargetFormat) => void
}

const UploadedFileQueueList = (
    { queue, activeFeature, onRemove, handleSetResolution, handleSetDenoiseQuality, handleSetTargetFormat }: UploadedFilesQueueListProps
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
                        <span className="ml-auto">
                        {(file.processingType === 'Transcode' || file.processingType === 'Upscale') && (
                            <OptionDropdown 
                                options={
                                    resolutionOptions(file.processingType, file.sourceHeight).map(r => ({ value: r.label, label: r.label }))
                                }
                                value={file.resolution}
                                onChange={v => handleSetResolution(file.id, v)}
                                ariaLabel={`Target resolution for ${file.name}`}
                            />
                        )}
                        {file.processingType === 'Denoise' && (
                            <OptionDropdown 
                                options={DENOISE_QUALITIES.map(o => ({ value: o, label: o}))}
                                value={file.denoiseQuality ?? 'medium'}
                                onChange={v => handleSetDenoiseQuality(file.id, v)}
                                ariaLabel={`Selected denoise quality for ${file.name}`}
                            />
                        )}
                        {file.processingType === 'Convert' && (
                            <OptionDropdown 
                                options={TARGET_FORMATS.map(o => ({ value: o, label: o}))}
                                value={file.targetFormat ?? 'mp4'}
                                onChange={v => handleSetTargetFormat(file.id, v)}
                                ariaLabel={`Target format for ${file.name}`}
                            />
                        )}
                        </span>
                    </div>
                    
                    {/*NOTE: maybe rename this to be clearer */}
                    <PipelineLane file={file} processingType={activeFeature}/>
                </li>
            ))}         
        </ul>
    )
}

interface ProcessedVideosListProps {
    processedVideos: ProcessedVideo[]
    onRemove: (jobId: string) => void
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
                    key={file.jobId}
                    className="relative flex items-center gap-2.5 px-5 py-3 border-b border-line hover:bg-row transition-colors duration-150"
                >
                    <span className="absolute left-0 top-3 bottom-3 w-[2px] rounded-full bg-status-done"/>
                    <span className="flex flex-col gap-0.5 flex-1 min-w-0">
                        <span className="truncate font-mono text-meta text-fg-strong" title={outputFileName(file)}>
                            {outputFileName(file)}
                        </span>
                    </span>
                    <DownloadButton file={file}/>
                    <button
                        onClick={() => onRemove(file.jobId)}
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