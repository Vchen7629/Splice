//go:build integration

package kv

import (
	"shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// creates bucket with correct name and TTL
func TestCreateMsgProcessedKV(t *testing.T) {
	js, _ := test.SetupNats(t)

	kv := CreateMsgProcessedKV("transcode-chunk-job-processed", js, test.SilentLogger())

	require.NotNil(t, kv)
	assert.Equal(t, "transcode-chunk-job-processed", kv.Bucket())
}
