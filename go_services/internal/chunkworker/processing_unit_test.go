//go:build unit

package chunkworker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func videoHeight(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=height",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	require.NoError(t, err, "ffprobe failed on %s", path)
	height, err := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, err, "unexpected ffprobe output: %q", string(out))

	return height
}

func TestProcessChunk(t *testing.T) {
	t.Run("unknown chunk operation returns error", func(t *testing.T) {
		outputPath, err := processChunk(pipeline.VideoChunkMessage{Operation: "Unknown"})

		require.Error(t, err)
		assert.Empty(t, outputPath)
		assert.Equal(t, "unsupported chunk operation: Unknown", err.Error())
	})

	t.Run("missing chunk in storage returns ErrNotFound", func(t *testing.T) {
		outputPath, err := processChunk(pipeline.VideoChunkMessage{
			JobID:      "job-404",
			Operation:  "transcode",
			StorageURL: test.NotFoundStorageURL(t) + "/job-404/video_chunk_0000.mkv",
		})

		require.ErrorIs(t, err, storage.ErrNotFound)
		assert.Empty(t, outputPath)
	})
}

func TestTranscodeVideo(t *testing.T) {
	baseURL := test.ServeTestChunks(t, 1)

	t.Run("writes output to chunk-named path", func(t *testing.T) {
		chunkName := "job-fmt"
		outDir := filepath.Join("/tmp/transcode", "temp-processed-"+chunkName)
		t.Cleanup(func() { os.RemoveAll(outDir) })

		outputPath, err := transcodeVideo(baseURL+"/chunk.mkv", "720p", chunkName)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(outDir, chunkName+".mp4"), outputPath)
		assert.FileExists(t, outputPath)
	})

	t.Run("mkdir failure returns wrapped create output dir error", func(t *testing.T) {
		chunkName := "job-blocked"
		blockedDir := filepath.Join("/tmp/transcode", "temp-processed-"+chunkName)
		require.NoError(t, os.MkdirAll("/tmp/transcode", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		outputPath, err := transcodeVideo(baseURL+"/chunk.mkv", "720p", chunkName)

		require.Error(t, err)
		assert.Empty(t, outputPath)
		assert.Contains(t, err.Error(), "create output transcode dir error")
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})

	t.Run("ffmpeg processing timeout returns context deadline exceeded", func(t *testing.T) {
		orig := ffmpegTimeout
		ffmpegTimeout = time.Microsecond
		t.Cleanup(func() { ffmpegTimeout = orig })

		_, err := transcodeVideo(baseURL+"/chunk.mkv", "720p", "job-timeout")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out")
	})

	errorTests := []struct {
		name string
		file string
	}{
		{name: "missing input returns ffmpeg error", file: "/missing.mkv"},
		{name: "non-video input returns ffmpeg error", file: "/garbage.mkv"},
	}
	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			chunkName := "job-err"
			t.Cleanup(func() { os.RemoveAll(filepath.Join("/tmp/transcode", "temp-processed-"+chunkName)) })

			_, err := transcodeVideo(baseURL+tc.file, "720p", chunkName)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}

	t.Run("transcodes to target resolution", func(t *testing.T) {
		chunkName := "job-resolution-1080p"
		t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + chunkName) })

		outputPath, err := transcodeVideo(baseURL+"/chunk.mkv", "1080p", chunkName)

		require.NoError(t, err)
		assert.Equal(t, 1080, videoHeight(t, outputPath))
	})
}

func TestDenoiseVideo(t *testing.T) {
	baseURL := test.ServeTestChunks(t, 1)

	t.Run("unsupported quality returns error", func(t *testing.T) {
		chunkName := "job-bad-quality"
		t.Cleanup(func() { os.RemoveAll("/tmp/denoise/temp-processed-" + chunkName) })

		outputPath, err := denoiseVideo("http://localhost:1/chunk.mkv", "boom", chunkName)

		require.Error(t, err)
		assert.Empty(t, outputPath)
		assert.Equal(t, "unsupported quality selected: boom", err.Error())
	})

	t.Run("writes output to chunk-named path", func(t *testing.T) {
		chunkName := "job-fmt"
		outDir := filepath.Join("/tmp/denoise", "temp-processed-"+chunkName)
		t.Cleanup(func() { os.RemoveAll(outDir) })

		outputPath, err := denoiseVideo(baseURL+"/chunk.mkv", "light", chunkName)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(outDir, chunkName+".mp4"), outputPath)
		assert.FileExists(t, outputPath)
	})

	t.Run("mkdir failure returns wrapped create output dir error", func(t *testing.T) {
		chunkName := "job-blocked"
		blockedDir := filepath.Join("/tmp/denoise", "temp-processed-"+chunkName)
		require.NoError(t, os.MkdirAll("/tmp/denoise", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		outputPath, err := denoiseVideo(baseURL+"/chunk.mkv", "light", chunkName)

		require.Error(t, err)
		assert.Empty(t, outputPath)
		assert.Contains(t, err.Error(), "create output denoise dir error")
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})

	t.Run("ffmpeg processing timeout returns context deadline exceeded", func(t *testing.T) {
		orig := ffmpegTimeout
		ffmpegTimeout = time.Microsecond
		t.Cleanup(func() { ffmpegTimeout = orig })

		_, err := denoiseVideo(baseURL+"/chunk.mkv", "light", "job-timeout")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out")
	})

	errorTests := []struct {
		name string
		file string
	}{
		{name: "missing input returns ffmpeg error", file: "/missing.mkv"},
		{name: "non-video input returns ffmpeg error", file: "/garbage.mkv"},
	}
	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			chunkName := "job-err"
			t.Cleanup(func() { os.RemoveAll(filepath.Join("/tmp/denoise", "temp-processed-"+chunkName)) })

			_, err := denoiseVideo(baseURL+tc.file, "light", chunkName)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}
}
