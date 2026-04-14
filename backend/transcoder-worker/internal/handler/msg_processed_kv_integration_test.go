//go:build integration

package handler

import (
	"testing"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// creates bucket with correct name and TTL
func TestCreateMsgProcessedKV(t *testing.T) {
	js, _ := test.SetupNats(t)

	kv := CreateMsgProcessedKV(js, test.SilentLogger())

	require.NotNil(t, kv)
	assert.Equal(t, "transcode-chunk-job-processed", kv.Bucket())
}
