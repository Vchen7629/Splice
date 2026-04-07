import { useRef } from 'react'
import FileUploadDropZone from './components/fileUploadDropZone'
import Header from './components/header'
import VideoUploadQueue from './components/videoUploadQueue'
import { useUploadQueue, type UploadedVideo } from './hooks/useUploadQueue'

let nextId = 0

function App() {
  const { uploadedVideos, setUploadedVideos, removeUploadedVideo, startVideoUploads } = useUploadQueue()
  const fileMap = useRef<Map<number, File>>(new Map())

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

    setUploadedVideos(prev => [...prev, ...newVideos])
  }

  function handleRemove(id: number) {
    fileMap.current.delete(id)
    removeUploadedVideo(id)
  }

  return (
    <>
      <Header />
      <main className="flex justify-center flex-1 w-full items-center px-6 py-6 gap-5 max-w-[80%] mx-auto">
        <FileUploadDropZone onFiles={handleFiles}/>
        <VideoUploadQueue 
          videos={uploadedVideos}
          setVideos={setUploadedVideos}
          onRemove={handleRemove}
          onStartUploads={() => startVideoUploads(fileMap.current)}
        />
      </main>
    </>
  )
}

export default App
