//go:build integration

package recombiner_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"splice.com/go_services/internal/recombiner"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeVideoChunk generates a minimal black H.264 video of the given duration
// (seconds) at path using ffmpeg's lavfi source.
func makeVideoChunk(t *testing.T, path string, seconds int) {
	t.Helper()
	cmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=320x240:d=%d", seconds),
		"-c:v", "libx264",
		"-t", strconv.Itoa(seconds),
		"-y",
		path,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to generate test video chunk: %s", out)
}

// videoDuration returns the duration of a video file in seconds using ffprobe.
func videoDuration(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	require.NoError(t, err, "ffprobe failed on %s", path)
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	require.NoError(t, err)
	return d
}

func TestCombineChunks(t *testing.T) {
	t.Run("produces output file", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-full") })

		chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
		chunk1 := filepath.Join(chunkDir, "chunk-1.mp4")
		makeVideoChunk(t, chunk0, 1)
		makeVideoChunk(t, chunk1, 1)

		outputPath, err := recombiner.CombineChunks("job-full", map[int]string{0: chunk0, 1: chunk1})

		require.NoError(t, err)
		info, statErr := os.Stat(outputPath)
		require.NoError(t, statErr, "output.mp4 should exist")
		assert.Greater(t, info.Size(), int64(0), "output.mp4 should be non-empty")
	})

	t.Run("output path is correctly formed", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/my-job") })

		chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
		makeVideoChunk(t, chunk0, 1)

		outputPath, err := recombiner.CombineChunks("my-job", map[int]string{0: chunk0})

		require.NoError(t, err)
		assert.Equal(t, "/tmp/jobs/my-job/output.mp4", outputPath)
	})

	t.Run("single chunk completes without error", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-single") })

		chunk0 := filepath.Join(chunkDir, "only-chunk.mp4")
		makeVideoChunk(t, chunk0, 1)

		outputPath, err := recombiner.CombineChunks("job-single", map[int]string{0: chunk0})

		require.NoError(t, err)
		_, statErr := os.Stat(outputPath)
		assert.NoError(t, statErr)
	})

	t.Run("all chunks are included in output", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-order") })

		// chunks have durations 1s, 2s, 3s — total 6s
		durations := map[int]int{0: 1, 1: 2, 2: 3}
		chunks := make(map[int]string, len(durations))
		var totalSeconds float64
		for idx, d := range durations {
			p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
			makeVideoChunk(t, p, d)
			chunks[idx] = p
			totalSeconds += float64(d)
		}

		outputPath, err := recombiner.CombineChunks("job-order", chunks)

		require.NoError(t, err)
		got := videoDuration(t, outputPath)
		assert.InDelta(t, totalSeconds, got, 0.1, "output duration should equal sum of all chunk durations")
	})

	t.Run("non-contiguous indices are all included", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-gaps") })

		indices := map[int]int{0: 1, 5: 2, 10: 1}
		chunks := make(map[int]string, len(indices))
		var totalSeconds float64
		for idx, d := range indices {
			p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
			makeVideoChunk(t, p, d)
			chunks[idx] = p
			totalSeconds += float64(d)
		}

		outputPath, err := recombiner.CombineChunks("job-gaps", chunks)

		require.NoError(t, err)
		got := videoDuration(t, outputPath)
		assert.InDelta(t, totalSeconds, got, 0.1)
	})

	t.Run("non-zero-based indices are all included", func(t *testing.T) {
		chunkDir := t.TempDir()
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-nonzero") })

		indices := map[int]int{3: 1, 4: 2, 5: 1}
		chunks := make(map[int]string, len(indices))
		var totalSeconds float64
		for idx, d := range indices {
			p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
			makeVideoChunk(t, p, d)
			chunks[idx] = p
			totalSeconds += float64(d)
		}

		outputPath, err := recombiner.CombineChunks("job-nonzero", chunks)

		require.NoError(t, err)
		got := videoDuration(t, outputPath)
		assert.InDelta(t, totalSeconds, got, 0.1)
	})
}
