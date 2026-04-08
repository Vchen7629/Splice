//go:build integration

package storage_test

import (
	"os"
	"testing"
	"video-recombiner/internal/storage"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
)

var sharedFilerUrl string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerUrl = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func TestCheckHealth(t *testing.T) {
	t.Run("storage health check fails when seedweedfs is unreachable", func(t *testing.T) {
		err := storage.CheckHealth("http://localhost:1", test.SilentLogger())

		assert.Error(t, err)
	})

	t.Run("storage health check passes when seedweedfs is reachable", func(t *testing.T) {
		err := storage.CheckHealth(sharedFilerUrl, test.SilentLogger())

		assert.NoError(t, err)
	})
}
