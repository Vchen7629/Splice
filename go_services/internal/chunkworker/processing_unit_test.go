//go:build unit

package chunkworker

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

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

const testVideoPath = "../shared/test/testvideo.mp4"

func TestProcessChunkErrorsOnUnknownChunkOp(t *testing.T) {
	outputPath, err := processChunk(&test.MockKV{}, VideoChunkMessage{Operation: "Unknown"}, test.SilentLogger())

	require.Error(t, err)
	assert.Equal(t, outputPath, "")
	assert.Equal(t, "unsupported chunk operation: Unknown", err.Error())
}

func TestTranscodeVideo(t *testing.T) {
	t.Run("mkdir failure returns create output dir error", func(t *testing.T) {
		blockedDir := "/tmp/transcode/temp-processed-job-blocked"
		require.NoError(t, os.MkdirAll("/tmp/transcode", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		path, err := transcodeVideo("/some/input.mp4", "720p", "job-blocked", test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "create output transcode dir error")
	})

	t.Run("mkdir failure wraps underlying OS error", func(t *testing.T) {
		blockedDir := "/tmp/transcode/temp-processed-job-blocked2"
		require.NoError(t, os.MkdirAll("/tmp/transcode", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		_, err := transcodeVideo("/some/input.mp4", "720p", "job-blocked2", test.SilentLogger())

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

	resolutionTests := []struct {
		name             string
		targetResolution string
		expectedHeight   int
	}{
		{
			name:             "downscales video to target resolution",
			targetResolution: "480p",
			expectedHeight:   480,
		},
		{
			name:             "upscales video to target resolution",
			targetResolution: "1080p",
			expectedHeight:   1080,
		},
	}

	for _, tc := range resolutionTests {
		t.Run(tc.name, func(t *testing.T) {
			jobID := "job-resolution-" + tc.targetResolution
			t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + jobID) })

			outputPath, err := transcodeVideo(testVideoPath, tc.targetResolution, jobID, test.SilentLogger())

			require.NoError(t, err)
			assert.FileExists(t, outputPath)
			assert.Equal(t, tc.expectedHeight, videoHeight(t, outputPath))
		})
	}

	t.Run("creates output directory", func(t *testing.T) {
		jobID := "job-dir"
		t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + jobID) })

		_, err := transcodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.DirExists(t, "/tmp/transcode/temp-processed-"+jobID)
	})

	t.Run("output path follows expected structure", func(t *testing.T) {
		jobID := "job-fmt"
		t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + jobID) })

		outputPath, err := transcodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/transcode/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
	})

	t.Run("normalizes non-mp4 source extension to mp4 output", func(t *testing.T) {
		jobID := "job-webm-src"
		t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + jobID) })

		outputPath, err := transcodeVideo("../shared/test/testvideo.webm", "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/transcode/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
		assert.FileExists(t, outputPath)
	})

	errorTests := []struct {
		name     string
		filePath string
		jobID    string
	}{
		{
			name:     "missing input file returns ffmpeg error",
			filePath: "/nonexistent/video.mp4",
			jobID:    "job-missing",
		},
		{
			name: "non-video input returns ffmpeg error",
			filePath: func() string {
				f := "/tmp/fake.mp4"
				_ = os.WriteFile(f, []byte("not a video"), 0644)
				return f
			}(),
			jobID: "job-fakevideo",
		},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { os.RemoveAll("/tmp/transcode/temp-processed-" + tc.jobID) })

			_, err := transcodeVideo(tc.filePath, "720p", tc.jobID, test.SilentLogger())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}
}

func TestDenoiseVideo(t *testing.T) {
	t.Run("mkdir failure returns create output dir error", func(t *testing.T) {
		blockedDir := "/tmp/denoise/temp-processed-job-blocked"
		require.NoError(t, os.MkdirAll("/tmp/denoise", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		path, err := denoiseVideo("/some/input.mp4", "job-blocked", test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "create output denoise dir error")
	})

	t.Run("mkdir failure wraps underlying OS error", func(t *testing.T) {
		blockedDir := "/tmp/denoise/temp-processed-job-blocked2"
		require.NoError(t, os.MkdirAll("/tmp/denoise", 0755))
		require.NoError(t, os.WriteFile(blockedDir, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove(blockedDir) })

		_, err := denoiseVideo("/some/input.mp4", "job-blocked2", test.SilentLogger())

		require.Error(t, err)
		var pathErr *os.PathError
		assert.ErrorAs(t, err, &pathErr)
	})

	t.Run("video ffmpeg processing timeout returns context deadline exceeded", func(t *testing.T) {
		orig := ffmpegTimeout
		ffmpegTimeout = time.Microsecond
		t.Cleanup(func() { ffmpegTimeout = orig })

		_, err := denoiseVideo("in.mp4", "job-timeout", test.SilentLogger())

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out")
	})

	t.Run("creates output directory", func(t *testing.T) {
		jobID := "job-dir"
		t.Cleanup(func() { os.RemoveAll("/tmp/denoise/temp-processed-" + jobID) })

		_, err := denoiseVideo(testVideoPath, jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.DirExists(t, "/tmp/denoise/temp-processed-"+jobID)
	})

	t.Run("output path follows expected structure", func(t *testing.T) {
		jobID := "job-fmt"
		t.Cleanup(func() { os.RemoveAll("/tmp/denoise/temp-processed-" + jobID) })

		outputPath, err := denoiseVideo(testVideoPath, jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/denoise/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
	})

	t.Run("normalizes non-mp4 source extension to mp4 output", func(t *testing.T) {
		jobID := "job-webm-src"
		t.Cleanup(func() { os.RemoveAll("/tmp/denoise/temp-processed-" + jobID) })

		outputPath, err := denoiseVideo("../shared/test/testvideo.webm", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/denoise/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
		assert.FileExists(t, outputPath)
	})

	errorTests := []struct {
		name     string
		filePath string
		jobID    string
	}{
		{
			name:     "missing input file returns ffmpeg error",
			filePath: "/nonexistent/video.mp4",
			jobID:    "job-missing",
		},
		{
			name: "non-video input returns ffmpeg error",
			filePath: func() string {
				f := "/tmp/fake.mp4"
				_ = os.WriteFile(f, []byte("not a video"), 0644)
				return f
			}(),
			jobID: "job-fakevideo",
		},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { os.RemoveAll("/tmp/denoise/temp-processed-" + tc.jobID) })

			_, err := denoiseVideo(tc.filePath, tc.jobID, test.SilentLogger())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}
}
