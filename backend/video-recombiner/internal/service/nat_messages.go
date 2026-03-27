package service

type ChunkCompleteMessage struct {
	JobID       string `json:"job_id"`
	ChunkIndex  int    `json:"chunk_index"`
	TotalChunks int    `json:"total_chunks"`
	OutputPath  string `json:"output_path"`
}

type VideoProcessingCompleteMessage struct {
	JobID       string `json:"job_id"`
}