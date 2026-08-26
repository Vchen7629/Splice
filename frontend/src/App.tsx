import { useRef, useState } from 'react'
import { Toaster } from 'sonner'
import { useVideoQueueStore } from './state/videoQueue'
import { useUploadQueue } from './hooks/useUploadQueue'
import { useJobEvents } from './hooks/useJobEvents'
import type { ProcessingType, UploadedFile } from './types/file'
import { defaultResolution, getVideoResolution } from './utils/videoResolution'
import { useTheme } from './hooks/useTheme'
import { useFileDrop } from './hooks/useFileDrop'
import AppLayout from './components/layout/AppLayout'
import Header from './components/layout/Header'
import Sidebar from './components/layout/Sidebar'
import ModeNav from './components/file/ModeNav'
import { FileOutputList, UploadedFileQueueList } from './components/file/InputOutputList'
import FileUploadPanel from './components/file/UploadPanel'

let nextId = 0

function App() {
  const { uploadedVideos, processedVideos, addVideos, removeProcessedVideo } = useVideoQueueStore()
  const [ activeFeature, setActiveFeature] = useState<ProcessingType>('Transcode')
  const { removeUploadedVideo, startVideoUploads } = useUploadQueue(activeFeature)
  const { resetVideo, setResolution } = useVideoQueueStore()
  const fileMap = useRef<Map<number, File>>(new Map())
  const { mode, toggleMode } = useTheme()
  const { isDragging, inputRef, browse, dropHandlers, handleInputChange } = useFileDrop(handleFiles)
  useJobEvents()

  const queue = uploadedVideos[activeFeature]
  const output = processedVideos[activeFeature]
  const showResolution = activeFeature !== 'Denoise'

  async function handleFiles(files: File[]) {
    const newFiles: UploadedFile[] = await Promise.all(files.map(async file => {
      const id = nextId++
      fileMap.current.set(id, file)

      let sourceHeight = 0
      try {
        const detected = await getVideoResolution(file)
        sourceHeight = detected.height
      } catch { /* leave as 0 — all resolutions will be shown */ }

      const resolution = defaultResolution(activeFeature, sourceHeight)

      return {
        id,
        name: file.name,
        size: file.size,
        resolution: resolution,
        sourceHeight,
        status: 'pending' as const,
        uploadProgress: 0,
        jobId: null,
      }
    }))
    addVideos(activeFeature, newFiles)
  }

  function handleRemove(processingType: ProcessingType, id: number) {
    fileMap.current.delete(id)
    removeUploadedVideo(processingType, id)
  }

  function handleSetResolution(id: number, resolution: string) {
      if (queue.find(v => v.id === id)?.status === 'error') resetVideo(activeFeature, id)
      setResolution(activeFeature, id, resolution)
  }

  return (
    <>
      <Toaster
        position='bottom-right'
        theme={mode}
        toastOptions={{
          unstyled: true,
          classNames: {
            toast:
              'flex items-start gap-3 w-full rounded-lg border border-line bg-panel px-4 py-3.5 shadow-lg',
            title: 'text-caption font-medium text-fg-strong',
            description: 'text-meta text-fg-muted mt-0.5',
            error: 'border-l-2 border-l-status-fail',
            success: 'border-l-2 border-l-status-done',
            closeButton: 'bg-panel border-line text-fg-muted hover:text-fg-strong',
          },
        }}
      />
      <AppLayout
        header={
          <Header
            darkLightMode={mode}
            onToggleMode={toggleMode}
            nav={<ModeNav activeFeature={activeFeature} onSelectFeature={setActiveFeature}/>}
          />
        }

        sidebar={
          <Sidebar
            queueCount={queue.length}
            outputCount={output.length}
            queueContent={
              <UploadedFileQueueList 
                queue={queue}
                activeFeature={activeFeature}
                showResolution={showResolution}
                onRemove={id => handleRemove(activeFeature, id)}
                handleSetResolution={handleSetResolution}
              />
            }
            outputContent={<FileOutputList output={output} onRemove={id => removeProcessedVideo(activeFeature, id)}/>}
          />
        }
      >
        <FileUploadPanel 
          activeFeature={activeFeature}
          queue={queue}
          outputCount={output.length}
          dropzone={{ isDragging, inputRef, browse, dropHandlers, handleInputChange }}
          onStartUploads={() => startVideoUploads(fileMap.current)}
        />
      </AppLayout>
    </>
  )
}

export default App
