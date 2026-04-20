//go:build unit

package kv_test

import (
	"errors"
	"shared/kv"
	"shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateJobStatus(t *testing.T) {
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
			err := kv.UpdateJobStatus(tc.kv, "video-upload", "job-1", test.SilentLogger())

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantKey, tc.kv.PutKey)
			}
		})
	}
}
