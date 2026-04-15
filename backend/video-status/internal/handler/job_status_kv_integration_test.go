//go:build integration

package handler

import (
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateJobStatusKV(t *testing.T) {
	kv := CreateJobStatusKV(sharedJS, test.SilentLogger())
	require.NotNil(t, kv)
	assert.Equal(t, "job-status", kv.Bucket())
}
