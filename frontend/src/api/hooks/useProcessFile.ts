import { AxiosError } from 'axios';
import type { createGenreEmbProps } from '../../shared/types/user';

interface processVideoReqProps = {

}

export const ProcessVide = {
    // create embeddings for selected genres during signup
    create_genre_embeddings: async({user_id, genres}: createGenreEmbProps) => {
        try {
            const response = await recommendations_api.post(`user/genre_embedding/${user_id}`, {
                genres: genres
            })

            return response.data
        } catch (error: unknown) {
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