import type { UploadedFile } from '../../types/file'

/** Status colour stays conventional across palettes — it carries meaning, not style. */
export const STATUS_LABEL: Record<UploadedFile['status'], { text: string; color: string; pulse: boolean }> = {
    pending:    { text: 'Queued',     color: 'text-status-idle',   pulse: false },
    uploading:  { text: 'Uploading',  color: 'text-status-upload', pulse: true  },
    processing: { text: 'Processing', color: 'text-status-work',   pulse: true  },
    complete:   { text: 'Done',       color: 'text-status-done',   pulse: false },
    error:      { text: 'Failed',     color: 'text-status-fail',   pulse: false },
    degraded:   { text: 'Degraded',   color: 'text-status-warn',   pulse: true  },
}

export const STATUS_BG: Record<UploadedFile['status'], string> = {
    pending:    'bg-status-idle',
    uploading:  'bg-status-upload',
    processing: 'bg-status-work',
    complete:   'bg-status-done',
    error:      'bg-status-fail',
    degraded:   'bg-status-warn',
}

export const StatusTag = ({ status }: { status: UploadedFile['status'] }) => {
    const label = STATUS_LABEL[status]
    
    return (
        <span className={`flex items-center gap-1 font-mono text-meta shrink-0 ${label.color}`}>
            {label.pulse && <span className="w-1 h-1 rounded-full bg-current animate-pulse" />}
            {label.text}
        </span>
    )
}