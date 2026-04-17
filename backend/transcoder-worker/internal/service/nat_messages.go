package service

// type struct for jetstream msg body
type VideoChunkMessage struct {
	JobID            string `json:"job_id"`
	ChunkIndex       int    `json:"chunk_index"`
	TotalChunks      int    `json:"total_chunks"`
	StorageURL       string `json:"storage_url"`
	TargetResolution string `json:"target_resolution"`
}
