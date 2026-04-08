//go:build integration

package service_test

import (
	"os"
	"testing"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVideoPath = "../test/test_video.mp4"

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

			outputPath, err := service.TranscodeVideo(testVideoPath, tc.targetResolution, jobID, test.SilentLogger())

			require.NoError(t, err)
			assert.FileExists(t, outputPath)
			assert.Equal(t, tc.expectedHeight, test.VideoHeight(t, outputPath))
		})
	}
}

func TestTranscodeOutput(t *testing.T) {
	t.Run("output path follows expected structure", func(t *testing.T) {
		jobID := "job-fmt"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

		outputPath, err := service.TranscodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "/tmp/temp-processed-"+jobID+"/test_video.mp4", outputPath)
	})

	t.Run("creates output directory", func(t *testing.T) {
		jobID := "job-dir"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-processed-" + jobID) })

		_, err := service.TranscodeVideo(testVideoPath, "720p", jobID, test.SilentLogger())

		require.NoError(t, err)
		assert.DirExists(t, "/tmp/temp-processed-"+jobID)
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

			_, err := service.TranscodeVideo(tc.filePath, "720p", tc.jobID, test.SilentLogger())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ffmpeg error")
		})
	}
}
