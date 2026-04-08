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
		blockedDir := "/tmp/temp-processed-job-blocked"
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		path, err := service.TranscodeVideo("/some/input.mp4", "720p", "job-blocked", test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "create output dir error")
	})

	t.Run("mkdir failure wraps underlying OS error", func(t *testing.T) {
		blockedDir := "/tmp/temp-processed-job-blocked2"
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		_, err := service.TranscodeVideo("/some/input.mp4", "720p", "job-blocked2", test.SilentLogger())

		require.Error(t, err)
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})
}
