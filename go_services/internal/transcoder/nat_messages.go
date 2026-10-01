package transcoder

import "splice.com/go_services/internal/shared/handler"

// type struct for jetstream msg body
type VideoChunkMessage struct {
	handler.ChunkRef
	TotalChunks      int    `json:"total_chunks"`
	StorageURL       string `json:"storage_url"`
	TargetResolution string `json:"target_resolution"`
}
