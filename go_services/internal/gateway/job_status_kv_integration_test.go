//go:build integration

package gateway

import (
	stest "splice.com/go_services/internal/shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateJobStatusKV(t *testing.T) {
	kv := CreateJobStatusKV(sharedJS, stest.SilentLogger())
	require.NotNil(t, kv)
	assert.Equal(t, "job-status", kv.Bucket())
}
