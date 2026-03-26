package service

// type struct for jetstream msg body
type VideoChunkMessage struct {
	JobID            string `json:"job_id"`
	ChunkIndex       int    `json:"chunk_index"`
	StoragePath      string `json:"storage_path"`
	TargetResolution string `json:"target_resolution"`
}

type ChunkCompleteMessage struct {
	JobID      string `json:"job_id"`
	ChunkIndex int    `json:"chunk_index"`
	OutputPath string `json:"output_path"`
}
