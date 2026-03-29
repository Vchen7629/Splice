import { Upload } from 'lucide-react'
import type { CSSProperties } from 'react'

const FORMATS = ['.mp4', '.mov', '.mkv', '.webm', '.avi']

/**
 * Drop zone, used to add video files for upload via drag & drop or click to browse.
 * TODO: wire onDragOver, onDragLeave, onDrop, onClick (hidden <input type="file">)
 */
const FileUploadDropZone = () => {
  return (
    <div
      className="relative flex flex-col bg-panel min-h-0 items-center justify-center w-[50%] border-1 border-line rounded-xl cursor-pointer gap-6 group"

      /* TODO: onDragOver, onDragLeave, onDrop handlers */
    >
      <CornerBrackets />

      <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-accent-bg border-1 border-accent-border">
        <Upload className='text-accent mb-1'/>
      </div>

      <div className="flex flex-col items-center gap-2 text-center">
        <span className="text-sm text-white font-medium">
          Drop video files here
        </span>
        <span className="text-xs leading-relaxed text-text-1">
          or
          <span className="ml-1 underline underline-offset-2 text-accent decoration-accent-border">
            click to browse
          </span>
        </span>
      </div>

      {/* Format pills */}
      <div className="flex items-center gap-1.5 flex-wrap justify-center px-10">
        {FORMATS.map(fmt => (
          <span
            key={fmt}
            className="text-xs px-2 py-0.5 rounded font-mono border-1 border-line bg-row-bg"
          >
            {fmt}
          </span>
        ))}
      </div>
    </div>
  )
}

/** Decorative corner brackets — add visual framing to the drop zone */
function CornerBrackets() {
  const s: CSSProperties = {
    position: 'absolute',
    width: 14,
    height: 14,
    borderColor: 'var(--border-hi)',
  }
  return (
    <>
      <span style={{ ...s, top: 10, left: 10, borderTop: '1px solid', borderLeft: '1px solid' }} />
      <span style={{ ...s, top: 10, right: 10, borderTop: '1px solid', borderRight: '1px solid' }} />
      <span style={{ ...s, bottom: 10, left: 10, borderBottom: '1px solid', borderLeft: '1px solid' }} />
      <span style={{ ...s, bottom: 10, right: 10, borderBottom: '1px solid', borderRight: '1px solid' }} />
    </>
  )
}

export default FileUploadDropZone
