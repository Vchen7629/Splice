//go:build integration

package handler

import (
	"testing"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// creates bucket with correct name and TTL
func TestCreateMsgRecievedKV(t *testing.T) {
	js, _ := test.SetupNats(t)

	kv := CreateMsgRecievedKV(js, test.SilentLogger())

	require.NotNil(t, kv)
	assert.Equal(t, "recombine-chunk-recieved", kv.Bucket())
}
