package handler

type VideoJobMessage struct {
	JobID            string `json:"job_id"`
	TargetResolution string `json:"target_resolution"`
	SourceResolution string `json:"source_resolution"`
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

// published on ephemeral core-NATS subject "progress.{job_id}" for progress tracking
type ProgressMessage struct {
	JobID    string `json:"job_id"`
	Stage    string `json:"stage"`
	Progress int    `json:"progress"`
}
