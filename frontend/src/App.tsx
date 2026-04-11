import { useRef } from 'react'
import FileUploadDropZone from './components/fileUploadDropZone'
import Header from './components/header'
import VideoUploadQueue from './components/videoUploadQueue'
import { Toaster } from 'sonner'
import { useVideoQueueStore } from './state/videoQueue'
import { useUploadQueue } from './hooks/useUploadQueue'
import { useJobPolling } from './hooks/useJobPolling'
import type { UploadedVideo } from './types/video'
import VideoHeader from './components/videoHeader'
import VideoUploadButton from './components/videoUploadButton'
import UploadVideoList from './components/uploadVideosList'
import ProcessedVideoList from './components/processedVideoList'

let nextId = 0

function App() {
  const { uploadedVideos, processedVideos, addVideos, removeProcessedVideo, setResolution } = useVideoQueueStore()
  const { removeUploadedVideo, startVideoUploads } = useUploadQueue()
  const fileMap = useRef<Map<number, File>>(new Map())
  useJobPolling()

  function handleFiles(files: File[]) {
    const newVideos: UploadedVideo[] = files.map(file => {
      const id = nextId++
      fileMap.current.set(id, file)
      return {
        id,
        name: file.name,
        size: file.size,
        resolution: '1080p',
        status: 'pending' as const,
        uploadProgress: 0,
        jobId: null
      }
    })
    addVideos(newVideos)
  }

  function handleRemove(id: number) {
    fileMap.current.delete(id)
    removeUploadedVideo(id)
  }

  return (
    <>
      <Header />
      <Toaster position='bottom-right'/>
      <main className="flex justify-center flex-1 w-full items-center px-6 py-6 gap-5 max-w-[80%] mx-auto">
        <FileUploadDropZone onFiles={handleFiles}/>
        <section className='flex flex-col flex-1 aspect-square justify-between'>
          <section className="flex flex-col w-full h-[60%] bg-panel border-1 border-line rounded-xl overflow-hidden">
            <VideoHeader videos={uploadedVideos} title='Processing Queue'/>
            <UploadVideoList videos={uploadedVideos} onRemove={handleRemove} onSetResolution={setResolution}/>
            <VideoUploadButton videos={uploadedVideos} onStartUploads={() => startVideoUploads(fileMap.current)}/>
          </section>
          <section className="flex flex-col w-full h-[35%] bg-panel border-1 border-line rounded-xl overflow-hidden">
            <VideoHeader videos={processedVideos} title='Processed Videos'/>
            <ProcessedVideoList processedVideos={processedVideos} onRemove={removeProcessedVideo}/>
          </section>
        </section>
      </main>
    </>
  )
}

export default App
