import { AxiosError } from "axios"
import { api } from "../lib/basePath"

export const VideoService = {
    // upload with XHR to show upload progress
    upload: (
        videoFile: File,
        targetResolution: string,
        onProgress: (pct: number) => void
    ): { promise: Promise<{ job_id: string }>; abort: () => void} => {
        let xhr: XMLHttpRequest
        const BASE_URL = 'http://localhost:8080'

        const promise = new Promise<{ job_id: string }>((resolve, reject) => {
            xhr = new XMLHttpRequest()

            const formData = new FormData()
            formData.append("video", videoFile)
            formData.append("target_resolution", targetResolution)

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

            xhr.open('POST', `${BASE_URL}/jobs`)
            xhr.send(formData)
        })

        return { promise, abort: () => xhr?.abort() }
    },

    status: async(id: string) => {
        try {
            const response = await api.get(`/jobs/${id}/status`)
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

    download: async(id: string) => {
        try {
            const response = await api.get(`/jobs/${id}/download`)
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
    }
}