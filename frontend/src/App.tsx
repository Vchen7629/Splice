import FileUploadDropZone from './components/fileUploadDropZone'
import Header from './components/header'
import VideoUploadQueue from './components/videoUploadQueue'

function App() {
  return (
    <>
      <Header />
      <main className="flex justify-between flex-1 w-full items-stretch px-6 py-6 gap-5">
        <FileUploadDropZone />
        <VideoUploadQueue />
      </main>
    </>
  )
}

export default App
