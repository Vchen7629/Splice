//go:build integration

package transcoder_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"splice.com/go_services/internal/shared/test"
	"splice.com/go_services/internal/transcoder"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

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

func TestTranscodeResolution(t *testing.T) {
	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jobID := "job-resolution-" + tc.targetResolution
			t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

			outputPath, err := transcoder.TranscodeVideo(testVideoPath, tc.targetResolution, jobID, test.SilentLogger())

			require.NoError(t, err)
			assert.FileExists(t, outputPath)
			assert.Equal(t, tc.expectedHeight, videoHeight(t, outputPath))
		})
	}
}

func TestTranscodeOutput(t *testing.T) {
	t.Run("output path follows expected structure", func(t *testing.T) {
		jobID := "job-fmt"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

		outputPath, err := transcoder.TranscodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
	})

	t.Run("creates output directory", func(t *testing.T) {
		jobID := "job-dir"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

		_, err := transcoder.TranscodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.DirExists(t, "/tmp/temp-processed-"+jobID)
	})

	t.Run("normalizes non-mp4 source extension to mp4 output", func(t *testing.T) {
		jobID := "job-webm-src"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

		outputPath, err := transcoder.TranscodeVideo("../shared/test/testvideo.webm", "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/temp-processed-"+jobID+"/testvideo.mp4", outputPath)
		assert.FileExists(t, outputPath)
	})
}

func TestTranscodeErrors(t *testing.T) {
	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + tc.jobID) })

			_, err := transcoder.TranscodeVideo(tc.filePath, "720p", tc.jobID, test.SilentLogger())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}
}
