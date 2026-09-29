package handler

import "fmt"

type ChunkRef struct {
	JobID      string `json:"job_id"`
	ChunkIndex int    `json:"chunk_index"`
}

// returns the key that identifies this chunk in the KV bucket, in form of <jobID>.<chunkIndex>
func (m *ChunkRef) ChunkKVKey() string {
	return fmt.Sprintf("%s.%d", m.JobID, m.ChunkIndex)
}

type VideoJobMessage struct {
	JobID            string `json:"job_id"`
	TargetResolution string `json:"target_resolution"`
	SourceResolution string `json:"source_resolution"`
	StorageURL       string `json:"storage_url"`
}

type ChunkCompleteMessage struct {
	ChunkRef
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
