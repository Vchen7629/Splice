import { useState } from 'react'
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
import { CancelledVideosList, ProcessedVideosList, UploadedFileQueueList } from './components/file/VideoDisplayList'
import FileUploadPanel from './components/file/UploadPanel'
import { useProcessedStore } from './state/processedVideos'
import { fileMap, nextFileId } from './state/fileRegistry'

function App() {
  const { videos, cancelled, addVideos, removeCancelled, resetVideo, setResolution } = useVideoQueueStore()
  const { processed, remove: removeProcessed} = useProcessedStore()
  const [ activeFeature, setActiveFeature] = useState<ProcessingType>('Transcode')
  const { mode, toggleMode } = useTheme()
  const { removeUploadedVideo, cancelVideo, startVideoUploads } = useUploadQueue()
  const { isDragging, inputRef, browse, dropHandlers, handleInputChange } = useFileDrop(handleFiles)
  useJobEvents()

  const queue = videos.filter(v => v.processingType === activeFeature)
  const processedVideo = processed.filter(v => v.processingType === activeFeature)
  const cancelledVideo = cancelled.filter(v => v.processingType === activeFeature)
  const showResolution = activeFeature !== 'Denoise'

  async function handleFiles(files: File[]) {
    const processingType = activeFeature
    
    const newFiles: UploadedFile[] = await Promise.all(files.map(async file => {
      const id = nextFileId()
      fileMap.set(id, file)

      let sourceHeight = 0
      try {
        const detected = await getVideoResolution(file)
        sourceHeight = detected.height
      } catch { /* leave as 0 — all resolutions will be shown */ }

      const resolution = defaultResolution(processingType, sourceHeight)

      return {
        id,
        processingType,
        name: file.name,
        size: file.size,
        resolution: resolution,
        sourceHeight,
        status: 'pending' as const,
        uploadProgress: 0,
        jobId: null,
      }
    }))
    addVideos(newFiles)
  }

  function handleRemove(id: number) {
    const file = videos.find(v => v.id === id)
    if (!file) return

    if (file.jobId === null || file.status === 'error') {
      removeUploadedVideo(id)
      return
    }

    cancelVideo(file)
  }

  function handleSetResolution(id: number, resolution: string) {
      if (queue.find(v => v.id === id)?.status === 'error') resetVideo(id)
      setResolution(id, resolution)
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
            processedCount={processedVideo.length}
            cancelledCount={cancelledVideo.length}
            queueContent={
              <UploadedFileQueueList 
                queue={queue}
                activeFeature={activeFeature}
                showResolution={showResolution}
                onRemove={handleRemove}
                handleSetResolution={handleSetResolution}
              />
            }
            processedContent={<ProcessedVideosList processedVideos={processedVideo} onRemove={removeProcessed}/>}
            cancelledContent={<CancelledVideosList cancelledVideos={cancelledVideo} onRemove={removeCancelled}/>}
          />
        }
      >
        <FileUploadPanel 
          activeFeature={activeFeature}
          queue={queue}
          processedCount={processedVideo.length}
          dropzone={{ isDragging, inputRef, browse, dropHandlers, handleInputChange }}
          onStartUploads={() => startVideoUploads(activeFeature)}
        />
      </AppLayout>
    </>
  )
}

export default App
