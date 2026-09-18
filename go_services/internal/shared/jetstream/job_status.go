package jetstream

type JobState string

const (
	StateProcessing JobState = "PROCESSING"
	StateComplete   JobState = "COMPLETE"
	StateCancelled  JobState = "CANCELLED"
	StateFailed     JobState = "FAILED"
	StateDegraded   JobState = "DEGRADED"
)

type JobStatus struct {
	State    JobState `json:"state"`
	Stage    string   `json:"stage"`
	Progress *int     `json:"progress,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func (m JobStatus) IsTerminal() bool {
	return m.State == "COMPLETE" || m.State == "FAILED" || m.State == "CANCELLED"
}
