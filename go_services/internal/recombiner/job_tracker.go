package recombiner

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

// record a completed chunk for a job from nats msgs, returns ready=true and a map of all chunk paths when all video
// chunks for the job has been recieved so the subscriber can trigger combiner.go and pass in the mapping to combine all.
// The job's state is kept (not deleted) once ready, so a retry of the triggering chunk after a failed combine attempt
// can still see every chunk's storage URL. Call Complete once the job has actually finished to release it.
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
		return true, state.chunks
	}

	return false, nil
}

// Complete releases a job's tracked state once it has actually finished (successfully combined and uploaded)
func (t *JobTracker) Complete(jobID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.jobs, jobID)
}
