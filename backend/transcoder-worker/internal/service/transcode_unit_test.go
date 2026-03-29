//go:build unit

package service_test

import (
	"os"
	"testing"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscodeVideo(t *testing.T) {
	t.Run("mkdir failure returns create output dir error", func(t *testing.T) {
		tmpDir := t.TempDir()
		test.BlockDirCreation(t, tmpDir)

		path, err := service.TranscodeVideo(test.BasePayload(), tmpDir, test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "create output dir error")
	})

	t.Run("mkdir failure wraps underlying OS error", func(t *testing.T) {
		tmpDir := t.TempDir()
		test.BlockDirCreation(t, tmpDir)

		_, err := service.TranscodeVideo(test.BasePayload(), tmpDir, test.SilentLogger())

		require.Error(t, err)
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})
}
