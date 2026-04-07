package service

type SceneSplitMessage struct {
	JobID            string `json:"job_id"`
	TargetResolution string `json:"target_resolution"`
	StorageURL       string `json:"storage_url"`
}

// JobCompleteMessage is published by the video-recombiner to jobs.complete when a job finishes.
type JobCompleteMessage struct {
	JobID string `json:"job_id"`
}
