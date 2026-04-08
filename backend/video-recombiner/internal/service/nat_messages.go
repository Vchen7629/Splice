package service

type ChunkCompleteMessage struct {
	JobID       string `json:"job_id"`
	ChunkIndex  int    `json:"chunk_index"`
	TotalChunks int    `json:"total_chunks"`
	StorageURL  string `json:"storage_url"`
}

type VideoProcessingCompleteMessage struct {
	JobID string `json:"job_id"`
}
