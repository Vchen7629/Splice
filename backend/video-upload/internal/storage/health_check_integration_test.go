//go:build integration

package storage_test

import (
	"testing"
	"video-upload/internal/storage"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
)

func TestCheckHealth(t *testing.T) {
	t.Run("storage health check fails when seedweedfs is unreachable", func(t *testing.T) {
		err := storage.CheckHealth("http://localhost:1")

		assert.Error(t, err)
	})

	t.Run("storage health check passes when seedweedfs is reachable", func(t *testing.T) {
		storageURL := test.SetupSeaweedFS(t)

		err := storage.CheckHealth(storageURL)

		assert.NoError(t, err)
	})
}
