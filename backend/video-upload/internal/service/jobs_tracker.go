package service

import "sync"

type CompletedJobs struct {
	mu   sync.RWMutex
	jobs map[string]bool
}

func NewCompletedJobs() *CompletedJobs {
	return &CompletedJobs{jobs: make(map[string]bool)}
}

// add a new completed job using jobID as key
func (c *CompletedJobs) AddJob(jobID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobs[jobID] = true
}

// check if the jobID is already marked as done in the mapping
func (c *CompletedJobs) IsDone(jobID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.jobs[jobID]
}
