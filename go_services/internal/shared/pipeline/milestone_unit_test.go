//go:build unit

package pipeline

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

func TestGetMilestone(t *testing.T) {

	t.Run("key that doesnt exist returns revision 0 and zero-value status", func(t *testing.T) {
		mockKV := &test.MockKV{}

		revision, status, err := GetMilestone(mockKV, "some-jobID")

		require.NoError(t, err)
		assert.Zero(t, revision)
		assert.Equal(t, JobStatus{}, status)
	})

	t.Run("error fetching from keyValue returns error", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("Some error")}

		revision, status, err := GetMilestone(mockKV, "some-jobID")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to fetch from kv: Some error")
		assert.Zero(t, revision)
		assert.Equal(t, JobStatus{}, status)
	})

	t.Run("returns error if its invalid json", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{[`),
		}

		revision, status, err := GetMilestone(mockKV, "job-1")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to unmarshal json: invalid character '[' looking for beginning of object key string")
		assert.Zero(t, revision)
		assert.Equal(t, JobStatus{}, status)
	})

	t.Run("returns revision and status for an existing entry", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound:         true,
			GetValue:         []byte(`{"state":"CANCELLED","stage":"chunk-worker"}`),
			GetEntryRevision: 5,
		}

		revision, status, err := GetMilestone(mockKV, "job-1")

		require.NoError(t, err)
		assert.Equal(t, uint64(5), revision)
		assert.Equal(t, JobStatus{State: "CANCELLED", Stage: "chunk-worker"}, status)
	})
}

func TestAdvanceMilestone(t *testing.T) {
	writePolicyTests := []struct {
		name      string
		current   *test.MockKV
		newStatus JobStatus
		wantWrite bool
	}{
		{
			name:      "creates entry when job has no milestone yet",
			current:   &test.MockKV{},
			newStatus: JobStatus{State: "PROCESSING", Stage: "chunk-worker"},
			wantWrite: true,
		},
		{
			name:      "advances when new stage is ahead of current",
			current:   &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: []byte(`{"state":"PROCESSING","stage":"chunk-worker"}`)},
			newStatus: JobStatus{State: "PROCESSING", Stage: "video-recombiner"},
			wantWrite: true,
		},
		{
			name:      "no-ops when new stage is behind current",
			current:   &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: []byte(`{"state":"PROCESSING","stage":"video-recombiner"}`)},
			newStatus: JobStatus{State: "PROCESSING", Stage: "chunk-worker"},
			wantWrite: false,
		},
		{
			name:      "no-ops on terminal COMPLETE",
			current:   &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: []byte(`{"state":"COMPLETE","stage":""}`)},
			newStatus: JobStatus{State: "PROCESSING", Stage: "chunk-worker"},
			wantWrite: false,
		},
		{
			name:      "no-ops on terminal FAILED",
			current:   &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: []byte(`{"state":"FAILED","stage":"upload"}`)},
			newStatus: JobStatus{State: "PROCESSING", Stage: "chunk-worker"},
			wantWrite: false,
		},
		{
			name:      "no-ops on terminal CANCELLED",
			current:   &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: []byte(`{"state":"CANCELLED","stage":"upload"}`)},
			newStatus: JobStatus{State: "PROCESSING", Stage: "chunk-worker"},
			wantWrite: false,
		},
	}

	for _, tc := range writePolicyTests {
		t.Run(tc.name, func(t *testing.T) {
			err := AdvanceMilestone(tc.current, "job-1", tc.newStatus)

			require.NoError(t, err)
			wrote := tc.current.CreateKey != "" || tc.current.UpdateKey != ""
			assert.Equal(t, tc.wantWrite, wrote)
		})
	}

	// it shouldnt be mistaken for missing key
	t.Run("existing entry that decodes to an empty state must be updated at its revision", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: true, GetEntryRevision: 3, GetValue: []byte(`{}`)}

		err := AdvanceMilestone(mockKV, "job-1", JobStatus{State: "PROCESSING", Stage: "chunk-worker"})

		require.NoError(t, err)
		assert.Empty(t, mockKV.CreateKey)
		assert.Equal(t, "job-1", mockKV.UpdateKey)
		assert.Equal(t, uint64(3), mockKV.UpdateRevision)
	})

	chunkWorkerValue := []byte(`{"state":"PROCESSING","stage":"chunk-worker"}`)

	errorTests := []struct {
		name   string
		mockKV *test.MockKV
	}{
		{"Get fails", &test.MockKV{GetErr: errors.New("kv unavailable")}},
		{"Create fails", &test.MockKV{CreateErr: errors.New("create failed")}},
		{"Update fails", &test.MockKV{GetFound: true, GetEntryRevision: 1, GetValue: chunkWorkerValue, UpdateErr: errors.New("update failed")}},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			newStatus := JobStatus{State: "PROCESSING", Stage: "video-recombiner"}

			err := AdvanceMilestone(tc.mockKV, "job-1", newStatus)

			require.Error(t, err)
			assert.ErrorContains(t, err, "failed")
		})
	}
}

func TestTerminateIfCancelled(t *testing.T) {
	t.Run("Naks and returns correct shouldCancel and stopJob when GetMilestoneKV returns an err", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("kv unavailable")}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.False(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.TermCalled)
	})

	t.Run("Returns false for both when job isnt cancelled", func(t *testing.T) {
		mockKV := &test.MockKV{}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.False(t, shouldCancel)
		assert.False(t, stopJob)
		assert.False(t, msg.NakCalled)
		assert.False(t, msg.TermCalled)
	})

	t.Run("Returns true when msg termination fails", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"chunk-worker"}`),
		}
		msg := &test.MockMsg{TermErr: errors.New("term failed")}

		shouldCancel, stopJob := TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.True(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.TermCalled)
	})

	t.Run("Returns true when msg Term is successful", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"chunk-worker"}`),
		}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.True(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.NakCalled)
	})
}
