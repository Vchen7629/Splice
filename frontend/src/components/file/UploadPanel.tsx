import type { ProcessingType, UploadedFile } from "../../types/file";
import { ALLOWED_EXTENSIONS } from "../../utils/fileUpload";
import PipelineLane from "./PipelineProgress";
import type { useFileDrop } from "../../hooks/useFileDrop";
import { STATUS_LABEL } from "./StatusStyles";

interface FileUploadPanelProps {
    activeFeature: ProcessingType
    queue: UploadedFile[]
    outputCount: number
    dropzone: ReturnType<typeof useFileDrop>
    onStartUploads: () => void
}

const ACTION_VERB: Record<ProcessingType, string> = {
    Transcode: 'Transcode',
    Upscale:   'Upscale',
    Denoise:   'Denoise',
    Convert:   'Convert',
}


/** Drop surface for user to add their file files to upload for */
const FileUploadPanel = ({ activeFeature, queue, outputCount, dropzone, onStartUploads }: FileUploadPanelProps) => {
    const { isDragging, inputRef, browse, dropHandlers, handleInputChange } = dropzone
    const pendingCount = queue.filter(v => v.status === 'pending').length
    const verb = ACTION_VERB[activeFeature]

    return (
        <section
            {...dropHandlers}
            onClick={browse}
            className={`relative flex flex-col items-center justify-center flex-1 min-w-0 gap-9 cursor-pointer
                  transition-colors duration-200 ${isDragging ? 'bg-accent-soft' : ''}`}
        >
            <input
                ref={inputRef}
                type="file"
                accept={ALLOWED_EXTENSIONS.join(',')}
                multiple
                className="hidden"
                onChange={handleInputChange}
            />

            {isDragging && (
                <span className="absolute inset-5 rounded-lg border border-dashed border-accent-line pointer-events-none"/>
            )}

            <QueueSummary queue={queue} outputCount={outputCount} verb={verb}/>
            <ActiveJobsPreview queue={queue} processingType={activeFeature}/>

            <div className="flex flex-col items-center gap-3">
                <span className="w-12 h-px bg-line-strong"/>
                <span className="text-caption text-fg-muted">
                    {isDragging ? 'Release to add files' : (
                        <>
                            Drop file files anywhere here, or{' '}
                            <button
                                onClick={e => { e.stopPropagation(); browse() }}
                                className="underline underline-offset-2 text-fg-strong decoration-line-strong hover:decoration-fg-strong"
                            >
                                browse
                            </button>
                        </>
                    )}
                </span>
                <span className="font-mono text-eyebrow uppercase text-fg-faint">
                    {ALLOWED_EXTENSIONS.map(e => e.slice(1)).join(' · ')}
                </span>
            </div>

            {pendingCount > 0 && (
                <button
                    onClick={e => { e.stopPropagation(); onStartUploads() }}
                    className="px-7 h-11 rounded-lg text-body font-medium bg-accent-solid text-on-accent hover:bg-accent-hover transition-colors duration-150"
                >
                    {verb} {pendingCount} {pendingCount === 1 ? 'file' : 'files'} →
                </button>
            )}
        </section>
    )
}

/**
 * Shows N files ready/in progress/finished
 */
const QueueSummary = ({ queue, outputCount, verb }: { queue: UploadedFile[]; outputCount: number; verb: string }) => {
    const working = queue.filter(v => v.status === 'processing' || v.status === 'uploading' || v.status === 'degraded')
    const pending = queue.filter(v => v.status === 'pending')

    const [count, line] = working.length > 0
        ? [working.length, working.length === 1 ? 'file in progress' : 'files in progress']
        : pending.length > 0
            ? [pending.length, pending.length === 1 ? `file ready to ${verb.toLowerCase()}` : `files ready to ${verb.toLowerCase()}`]
            : [outputCount, outputCount === 1 ? 'file finished' : 'files finished']

    if (count === 0) {
        return (
            <div className="flex flex-col items-center gap-3">
                <span className="text-display font-semibold text-fg-strong">{verb} file</span>
                <span className="text-body text-fg-muted max-w-[44ch] text-center leading-relaxed">
                    Splice splits each file by scene, processes the pieces in parallel, then
                    recombines them.
                </span>
            </div>
        )
    }

    return (
        <div className="flex flex-col items-center gap-2">
            <span className="text-hero font-bold text-accent tabular-nums">{count}</span>
            <span className="text-lead text-fg-muted">{line}</span>
        </div>
    )
}

/**
 * Full-size pipeline lanes for whatever is running, so progress is legible from across the room. Silent when nothing is in flight.
 */
const ActiveJobsPreview = ({ queue, processingType }: { queue: UploadedFile[]; processingType: ProcessingType }) => {
    const active = queue.filter(v => v.status === 'processing' || v.status === 'uploading' || v.status === 'degraded')

    if (active.length === 0) return null

    return (
        <div className="flex flex-col gap-4 w-full max-w-[440px] px-6" onClick={e => e.stopPropagation()}>
            {active.slice(0, 3).map(file => (
                <div key={file.id} className="flex flex-col gap-1.5">
                    <div className="flex items-baseline gap-2">
                        <span className="flex-1 min-w-0 truncate font-mono text-meta text-fg" title={file.name}>
                            {file.name}
                        </span>
                        <StatusTag status={file.status} />
                    </div>
                    <PipelineLane file={file} processingType={processingType} />
                </div>
            ))}

            {active.length > 3 && (
                <span className="font-mono text-eyebrow uppercase text-fg-faint">
                    +{active.length - 3} more in the queue
                </span>
            )}
        </div>
    )
}

const StatusTag = ({ status }: { status: UploadedFile['status'] }) => {
    const label = STATUS_LABEL[status]

    return (
        <span className={`flex items-center gap-1 font-mono text-meta shrink-0 ${label.color}`}>
            {label.pulse && <span className="w-1 h-1 rounded-full bg-current animate-pulse" />}
            {label.text}
        </span>
    )
}

export default FileUploadPanel