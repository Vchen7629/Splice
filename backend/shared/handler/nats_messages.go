package handler

type VideoJobMessage struct {
	JobID            string `json:"job_id"`
	TargetResolution string `json:"target_resolution"`
	StorageURL       string `json:"storage_url"`
}

type ChunkCompleteMessage struct {
	JobID       string `json:"job_id"`
	ChunkIndex  int    `json:"chunk_index"`
	TotalChunks int    `json:"total_chunks"`
	StorageURL  string `json:"storage_url"`
}

type JobCompleteMessage struct {
	JobID string `json:"job_id"`
}
