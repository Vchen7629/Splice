package service

import "sync"

type jobState struct {
	chunks      map[int]string
	totalChunks int
}

type JobTracker struct {
	mu   sync.Mutex
	jobs map[string]*jobState
}

func NewJobTracker() *JobTracker {
	return &JobTracker{
		jobs: make(map[string]*jobState),
	}
}

// record a completed chunk for a job from nats msgs, returns read=true and a map of all chunk paths when all video
// chunks for the job has been recieved so the subscriber can trigger combiner.go and pass in the mapping to combine all
func (t *JobTracker) Add(jobID string, chunkIndex int, storageURL string, totalChunks int) (ready bool, chunks map[int]string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.jobs[jobID]
	if !ok {
		state = &jobState{
			chunks:      make(map[int]string),
			totalChunks: totalChunks,
		}
		t.jobs[jobID] = state
	}

	state.chunks[chunkIndex] = storageURL

	if len(state.chunks) == state.totalChunks {
		chunks = state.chunks
		delete(t.jobs, jobID)
		return true, chunks
	}

	return false, nil
}
