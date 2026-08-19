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
