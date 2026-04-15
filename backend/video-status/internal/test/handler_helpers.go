//go:build integration

package test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// JobStatus mirrors handler.JobStatus for KV assertion helpers.
// Kept minimal — only the fields needed for assertions.
type AssertJobStatus struct {
	State string `json:"state"`
	Stage string `json:"stage"`
	Error string `json:"error,omitempty"`
}

// AssertKVFailed polls the KV until the entry for jobID has state FAILED, then checks the error field contains wantErrContains.
func AssertKVFailed(t *testing.T, kv jetstream.KeyValue, jobID, wantErrContains string) {
	t.Helper()
	require.Eventually(t, func() bool {
		entry, err := kv.Get(context.Background(), jobID)
		if err != nil {
			return false
		}
		var s AssertJobStatus
		return json.Unmarshal(entry.Value(), &s) == nil && s.State == "FAILED"
	}, 5*time.Second, 100*time.Millisecond, "KV entry for %q never reached FAILED state", jobID)

	entry, err := kv.Get(context.Background(), jobID)
	require.NoError(t, err)
	var s AssertJobStatus
	require.NoError(t, json.Unmarshal(entry.Value(), &s))
	assert.Contains(t, s.Error, wantErrContains)
}

// AssertKVEmpty waits briefly then asserts the key is absent from the KV.
func AssertKVEmpty(t *testing.T, kv jetstream.KeyValue, jobID string) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	_, err := kv.Get(context.Background(), jobID)
	assert.True(t, errors.Is(err, jetstream.ErrKeyNotFound), "expected KV entry for %q to be absent, got err: %v", jobID, err)
}

// AssertKVComplete polls the KV until the entry for jobID has state COMPLETE.
func AssertKVComplete(t *testing.T, kv jetstream.KeyValue, jobID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		entry, err := kv.Get(context.Background(), jobID)
		if err != nil {
			return false
		}
		var s AssertJobStatus
		return json.Unmarshal(entry.Value(), &s) == nil && s.State == "COMPLETE"
	}, 5*time.Second, 100*time.Millisecond, "KV entry for %q never reached COMPLETE state", jobID)
}

func AssertKVDegraded(t *testing.T, kv jetstream.KeyValue, jobID, wantErrContains string) {
	t.Helper()
	require.Eventually(t, func() bool {
		entry, err := kv.Get(context.Background(), jobID)
		if err != nil {
			return false
		}

		var s AssertJobStatus
		return json.Unmarshal(entry.Value(), &s) == nil && s.State == "DEGRADED"
	}, 5*time.Second, 100*time.Millisecond, "KV entry for %q never reached DEGRADED state", jobID)

	entry, err := kv.Get(context.Background(), jobID)
	require.NoError(t, err)

	var s AssertJobStatus
	require.NoError(t, json.Unmarshal(entry.Value(), &s))
	assert.Contains(t, s.Error, wantErrContains)
}
