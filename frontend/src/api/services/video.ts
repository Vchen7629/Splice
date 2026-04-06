import { AxiosError } from "axios"
import { api } from "../lib/basePath"

export const VideoService = {
    upload: async({ videoFile, targetResolution}: any) => {
        const formData = new FormData
        formData.append("file", videoFile)
        try {
            const response = await api.post("/jobs", formData)
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