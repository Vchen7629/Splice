//go:build unit

package handler_test

import (
	"errors"
	"testing"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateJobStatusKV(t *testing.T) {
	tests := []struct {
		name    string
		kv      *test.MockKV
		wantErr bool
		wantKey string
	}{
		{
			name:    "success returns nil and writes job_id as key",
			kv:      &test.MockKV{},
			wantErr: false,
			wantKey: "job-1",
		},
		{
			name:    "KV Put error returns error",
			kv:      &test.MockKV{PutErr: errors.New("kv unavailable")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := handler.UpdateJobStatusKV(tc.kv, "job-1", test.SilentLogger())

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantKey, tc.kv.PutKey)
			}
		})
	}
}
