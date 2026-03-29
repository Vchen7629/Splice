//go:build integration

package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVideoPath = "../test/test_video.mp4"

func TestTranscodeResolution(t *testing.T) {
	t.Run("downscales video to target resolution", func(t *testing.T) {
		payload := service.VideoChunkMessage{
			JobID:            "test-job",
			ChunkIndex:       0,
			StoragePath:      testVideoPath,
			TargetResolution: "480p",
		}

		outputPath, err := service.TranscodeVideo(payload, t.TempDir(), test.SilentLogger())

		require.NoError(t, err)
		assert.FileExists(t, outputPath)
		assert.Equal(t, 480, test.VideoHeight(t, outputPath))
	})

	t.Run("upscales video to target resolution", func(t *testing.T) {
		payload := service.VideoChunkMessage{
			JobID:            "test-job",
			ChunkIndex:       0,
			StoragePath:      testVideoPath,
			TargetResolution: "1080p",
		}

		outputPath, err := service.TranscodeVideo(payload, t.TempDir(), test.SilentLogger())

		require.NoError(t, err)
		assert.FileExists(t, outputPath)
		assert.Equal(t, 1080, test.VideoHeight(t, outputPath))
	})
}

func TestTranscodeOutput(t *testing.T) {
	t.Run("output path follows expected structure", func(t *testing.T) {
		jobID := "test-job-fmt"
		chunkIndex := 7
		tmpDir := t.TempDir()
		payload := service.VideoChunkMessage{
			JobID:            jobID,
			ChunkIndex:       chunkIndex,
			StoragePath:      testVideoPath,
			TargetResolution: "720p",
		}

		outputPath, err := service.TranscodeVideo(payload, tmpDir, test.SilentLogger())

		require.NoError(t, err)
		expected := filepath.Join(tmpDir, "jobs", jobID, "transcoded", fmt.Sprintf("chunk_%03d.mp4", chunkIndex))
		assert.Equal(t, expected, outputPath)
	})

	t.Run("creates output directory", func(t *testing.T) {
		jobID := "test-job-dir"
		tmpDir := t.TempDir()
		payload := service.VideoChunkMessage{
			JobID:            jobID,
			ChunkIndex:       0,
			StoragePath:      testVideoPath,
			TargetResolution: "720p",
		}

		_, err := service.TranscodeVideo(payload, tmpDir, test.SilentLogger())

		require.NoError(t, err)
		assert.DirExists(t, filepath.Join(tmpDir, "jobs", jobID, "transcoded"))
	})
}

func TestTranscodeErrors(t *testing.T) {
	t.Run("missing input file returns ffmpeg error", func(t *testing.T) {
		payload := service.VideoChunkMessage{
			JobID:            "test-job",
			ChunkIndex:       0,
			StoragePath:      "/nonexistent/video.mp4",
			TargetResolution: "720p",
		}

		_, err := service.TranscodeVideo(payload, t.TempDir(), test.SilentLogger())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffmpeg error")
	})

	t.Run("non-video input returns ffmpeg error", func(t *testing.T) {
		tmpDir := t.TempDir()
		fakeVideo := filepath.Join(tmpDir, "fake.mp4")
		require.NoError(t, os.WriteFile(fakeVideo, []byte("not a video"), 0644))

		payload := service.VideoChunkMessage{
			JobID:            "test-job",
			ChunkIndex:       0,
			StoragePath:      fakeVideo,
			TargetResolution: "720p",
		}

		_, err := service.TranscodeVideo(payload, t.TempDir(), test.SilentLogger())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffmpeg error")
	})
}
