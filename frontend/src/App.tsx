import { useRef, useState } from 'react'
import FileUploadDropZone from './components/videoUploadPanel'
import { Toaster } from 'sonner'
import { useVideoQueueStore } from './state/videoQueue'
import { useUploadQueue } from './hooks/useUploadQueue'
import { useJobPolling } from './hooks/useJobPolling'
import type { ProcessingType, UploadedVideo } from './types/video'
import Sidebar from './components/navigation'
import VideoPanel from './components/videoDisplayPanel'
import { defaultResolution, getVideoResolution } from './utils/videoResolution'

let nextId = 0

function App() {
  const { uploadedVideos, processedVideos, addVideos, removeProcessedVideo } = useVideoQueueStore()
  const [ activeFeature, setActiveFeature] = useState<ProcessingType>('Transcode')
  const { removeUploadedVideo } = useUploadQueue(activeFeature)
  const fileMap = useRef<Map<number, File>>(new Map())
  useJobPolling()

  async function handleFiles(files: File[]) {
    const newVideos: UploadedVideo[] = await Promise.all(files.map(async file => {
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
    addVideos(activeFeature, newVideos)
  }

  function handleRemove(processingType: ProcessingType, id: number) {
    fileMap.current.delete(id)
    removeUploadedVideo(processingType, id)
  }

  return (
    <>
      <Toaster position='bottom-right'/>
      <div className="flex flex-1 w-full h-full overflow-hidden">
        <Sidebar activeFeature={activeFeature} onSelect={setActiveFeature} />
        <main className="flex flex-col lg:flex-row justify-center flex-1 items-center px-6 py-6 gap-5 max-w-[80%] mx-auto">
          <FileUploadDropZone onFiles={handleFiles}/>
          <VideoPanel
            uploadedVideos={uploadedVideos}
            processedVideos={processedVideos}
            onRemove={handleRemove}
            onRemoveProcessed={removeProcessedVideo}
            fileMap={fileMap}
            processingType={activeFeature}
          />
        </main>
      </div>
    </>
  )
}

export default App
