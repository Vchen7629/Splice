import { Upload } from 'lucide-react'
import { useRef, useState, type ChangeEvent, type CSSProperties, type DragEvent } from 'react'
import { ALLOWED_EXTENSIONS, CollectFilesFromDrop, FilterFiles } from '../utils/fileUpload'

/**
 * Drop zone, used to add video files for upload via drag & drop or click to browse.
 * TODO: wire onDragOver, onDragLeave, onDrop, onClick (hidden <input type="file">)
 */
const FileUploadDropZone = ({ onFiles }: { onFiles: (files: File[]) => void}) => {
  const [isDragging, setIsDragging] = useState<boolean>(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function handleDragOver(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setIsDragging(true)
  }

  function handleDragLeave() {
    setIsDragging(false)
  }

  async function handleDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setIsDragging(false)
    const files = await CollectFilesFromDrop(e)
    const { accepted } = FilterFiles(files)

    if (accepted.length > 0) onFiles(accepted)
  }

  function handleInputChange(e: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? [])
    
    const { accepted } = FilterFiles(files)
    if (accepted.length > 0) onFiles(accepted)

    e.target.value = '' // reseting so the same file can be readded
  }

  return (
    <div
      className={`relative flex flex-col min-h-0 items-center justify-center flex-1 aspect-square border-2
        rounded-xl cursor-pointer gap-6 group transition-all duration-150
        ${isDragging ? 'border-transparent bg-accent/10 scale-[1.01] shadow-[0_0_24px_rgba(var(--accent-rgb),0.25)]'
          : 'bg-panel border-line border-solid'
        }`}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <input 
        ref={inputRef}
        type="file"
        accept={ALLOWED_EXTENSIONS.join(',')}
        multiple
        className='hidden'
        onChange={handleInputChange}
      />
      <CornerBrackets active={isDragging} />

      <div className="flex items-center justify-center w-12 h-12 rounded-xl bg-accent-bg border-1 border-accent-border">
        <Upload className='text-accent mb-1'/>
      </div>

      <div className="flex flex-col items-center gap-2 text-center">
        <span className="text-sm text-white font-medium">
          Drop video files here
        </span>
        <span className="text-xs leading-relaxed text-text-1">
          or
          <button
            className="ml-1 underline underline-offset-2 text-accent decoration-accent-border"
            onClick={() => inputRef.current?.click()}
          >
            click to browse
          </button>
        </span>
      </div>

      {/* Format pills */}
      <div className="flex items-center gap-1.5 flex-wrap justify-center px-10">
        {ALLOWED_EXTENSIONS.map(fmt => (
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
const CornerBrackets = ({ active }: { active: boolean }) => {
  const s: CSSProperties = {
    position: 'absolute',
    width: active ? 18 : 14,
    height: active ? 18 : 14,
    borderColor: active ? 'var(--accent)' : 'var(--border-hi)',
    transition: 'all 0.15s ease',
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
