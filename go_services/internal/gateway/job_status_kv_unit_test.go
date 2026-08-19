//go:build unit

package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"
	//"video-status/internal/test"
	stest "splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJobStatusKV(t *testing.T) {
	tests := []struct {
		name       string
		kv         *MockKV
		wantStatus int
		wantErr    string
	}{
		{
			name:       "key not found returns 404",
			kv:         NewMockKV(),
			wantStatus: http.StatusNotFound,
			wantErr:    "job not found",
		},
		{
			name: "generic KV error returns 500",
			kv: func() *MockKV {
				m := NewMockKV()
				m.GetErr = errors.New("kv unavailable")
				return m
			}(),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to get job status",
		},
		{
			name: "success returns entry and 200",
			kv: func() *MockKV {
				m := NewMockKV()
				m.Seed("job-1", []byte(`{"state":"PROCESSING"}`))
				return m
			}(),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &KVHandler{logger: stest.SilentLogger(), kv: tc.kv}
			entry, code, err := h.getJobStatusKV(context.Background(), "job-1")

			assert.Equal(t, tc.wantStatus, code)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, entry)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, entry)
			}
		})
	}
}

func TestUpdateJobStatusKV(t *testing.T) {
	tests := []struct {
		name    string
		kv      *MockKV
		wantErr bool
	}{
		{name: "success returns nil", kv: NewMockKV(), wantErr: false},
		{
			name: "KV Put error returns error",
			kv: func() *MockKV {
				m := NewMockKV()
				m.PutErr = errors.New("kv unavailable")
				return m
			}(),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &KVHandler{logger: stest.SilentLogger(), kv: tc.kv}
			err := h.updateJobStatusKV(context.Background(), "job-1", JobStatus{State: StateProcessing, Stage: "scene-detector"})

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
