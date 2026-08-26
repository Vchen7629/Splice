import { AxiosError } from "axios"
import { GatewayApi, GATEWAY_BASE_URL } from "../lib/basePath"

export const VideoService = {
    // upload with XHR to show upload progress
    upload: (
        videoFile: File,
        targetResolution: string,
        sourceResolution: string,
        processingType: string,
        onProgress: (pct: number) => void
    ): { promise: Promise<{ job_id: string }>; abort: () => void} => {
        let xhr: XMLHttpRequest

        const promise = new Promise<{ job_id: string }>((resolve, reject) => {
            xhr = new XMLHttpRequest()

            const formData = new FormData()
            formData.append("video", videoFile)
            formData.append("target_resolution", targetResolution)
            formData.append("source_resolution", sourceResolution)
            formData.append("process_type", processingType)

            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
            })

            xhr.addEventListener('load', () => {
                if (xhr.status == 201) {
                    resolve(JSON.parse(xhr.responseText))
                } else {
                    reject(new Error(`Upload failed: ${xhr.status} ${xhr.statusText}`))
                }
            })
            xhr.addEventListener('error', () => reject(new Error('Network error during upload')))
            xhr.addEventListener('abort', () => reject(new DOMException('Upload cancelled', 'AbortError')))

            xhr.open('POST', `${GATEWAY_BASE_URL}/jobs/upload`)
            xhr.send(formData)
        })

        return { promise, abort: () => xhr?.abort() }
    },

    status: async(id: string) => {
        try {
            const response = await GatewayApi.get(`/jobs/${id}/status`)
            return response.data
        } catch (error) {
            if (error instanceof AxiosError) {
                console.error(error.response?.data || error.message);
                throw error;
            } else if (error instanceof Error) {
                console.error(error.message);
                throw error;
            } else {
                console.error(error);
                throw error;
            }
        }
    },

    connectEvents: (jobId: string): EventSource => 
        new EventSource(`${GATEWAY_BASE_URL}/jobs/${jobId}/events`),

    download: async(jobId: string, fileName: string) => {
        try {
            const response = await GatewayApi.post(`/jobs/download`, { job_id: jobId, file_name: fileName }, { responseType: 'blob' })
            return response.data as Blob
        } catch (error) {
            if (error instanceof AxiosError) {
                console.error(error.response?.data || error.message);
                throw error;
            } else if (error instanceof Error) {
                console.error(error.message);
                throw error;
            } else {
                console.error(error);
                throw error;
            }
        }
    }
}