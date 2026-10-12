//go:build unit

package transcoder

import (
	"context"
	"os"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscodeVideo(t *testing.T) {
	t.Run("mkdir failure returns create output dir error", func(t *testing.T) {
		blockedDir := "/tmp/temp-processed-job-blocked"
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		path, err := TranscodeVideo("/some/input.mp4", "720p", "job-blocked", test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "create output dir error")
	})

	t.Run("mkdir failure wraps underlying OS error", func(t *testing.T) {
		blockedDir := "/tmp/temp-processed-job-blocked2"
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		_, err := TranscodeVideo("/some/input.mp4", "720p", "job-blocked2", test.SilentLogger())

		require.Error(t, err)
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})

	t.Run("video ffmpeg processing timeout returns context deadline exceeded", func(t *testing.T) {
		orig := ffmpegTimeout
		ffmpegTimeout = time.Microsecond
		t.Cleanup(func() { ffmpegTimeout = orig })

		_, err := transcodeVideo("in.mp4", "720p", "job-timeout", test.SilentLogger())

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out")
	})
}
