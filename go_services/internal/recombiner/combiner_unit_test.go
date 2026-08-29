//go:build unit

package recombiner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generates a minimal block H.264 video of given duration at path
func makeVideoChunk(t *testing.T, path string, duration_s int) {
	t.Helper()
	cmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=320x240:d=%d", duration_s),
		"-c:v", "libx264",
		"-t", strconv.Itoa(duration_s),
		"-y",
		path,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to generate test video chunk: %s", out)
}

func TestCombineChunksErrors(t *testing.T) {
	t.Run("MkdirAll failure returns wrapped error", func(t *testing.T) {
		// Block /tmp/jobs/job-1 by pre-creating it as a file.
		blockingFile := "/tmp/jobs/job-1"
		require.NoError(t, os.MkdirAll(filepath.Dir(blockingFile), 0755))
		require.NoError(t, os.WriteFile(blockingFile, []byte{}, 0644))
		t.Cleanup(func() { os.Remove(blockingFile) })

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "create output dir error")
	})

	t.Run("manifest write failure returns wrapped error", func(t *testing.T) {
		outDir := "/tmp/jobs/job-1"
		require.NoError(t, os.MkdirAll(outDir, 0755))
		require.NoError(t, os.Chmod(outDir, 0555))
		t.Cleanup(func() {
			os.Chmod(outDir, 0755)
			os.RemoveAll(outDir)
		})

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "write manifest error")
	})

	t.Run("ffmpeg failure returns wrapped error", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-1") })

		_, err := CombineChunks("job-1", map[int]string{0: "/nonexistent/chunk.mp4"}, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffmpeg concat error")
	})

}

func TestManifest(t *testing.T) {
	t.Run("is sorted by chunk index regardless of map order", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-1") })
		chunks := map[int]string{
			2: "/fake/c.mp4",
			0: "/fake/a.mp4",
			1: "/fake/b.mp4",
		}

		_, err := CombineChunks("job-1", chunks, nil)
		require.Error(t, err) // ffmpeg fails on fake paths — expected

		raw, readErr := os.ReadFile("/tmp/jobs/job-1/manifest.txt")
		require.NoError(t, readErr, "manifest.txt should exist even when ffmpeg fails")

		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		require.Len(t, lines, 3)
		assert.Equal(t, "file '/fake/a.mp4'", lines[0])
		assert.Equal(t, "file '/fake/b.mp4'", lines[1])
		assert.Equal(t, "file '/fake/c.mp4'", lines[2])
	})

	t.Run("is written to /tmp/jobs/{jobID}/manifest.txt", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/my-job") })

		_, _ = CombineChunks("my-job", map[int]string{0: "/fake/chunk.mp4"}, nil)

		_, err := os.Stat("/tmp/jobs/my-job/manifest.txt")
		assert.NoError(t, err)
	})

	t.Run("empty chunks map writes an empty manifest", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-empty") })

		_, err := CombineChunks("job-empty", map[int]string{}, nil)
		require.Error(t, err) // ffmpeg fails with no inputs — expected

		raw, readErr := os.ReadFile("/tmp/jobs/job-empty/manifest.txt")
		require.NoError(t, readErr)
		assert.Empty(t, strings.TrimSpace(string(raw)))
	})

	t.Run("single chunk produces one file entry", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-1") })

		_, _ = CombineChunks("job-1", map[int]string{0: "/fake/only.mp4"}, nil)

		raw, err := os.ReadFile("/tmp/jobs/job-1/manifest.txt")
		require.NoError(t, err)
		assert.Equal(t, "file '/fake/only.mp4'\n", string(raw))
	})

	t.Run("a chunk that fails to probe does not block manifest write", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-unprobeable") })

		_, _ = CombineChunks("job-unprobeable", map[int]string{0: "/nonexistent/chunk.mp4"}, nil)

		_, err := os.Stat("/tmp/jobs/job-unprobeable/manifest.txt")
		assert.NoError(t, err)
	})
}

func TestProbeDurationSeconds(t *testing.T) {
	t.Run("returns video duration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "chunk.mp4")
		makeVideoChunk(t, path, 2)

		duration, err := probeDurationSeconds(path)

		require.NoError(t, err)
		assert.InDelta(t, 2.0, duration, 0.2)
	})

	t.Run("nonexistent file returns wrapped error", func(t *testing.T) {
		_, err := probeDurationSeconds("/idk")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffprobe duration error")
	})
}

func TestCombineChunksProgress(t *testing.T) {
	t.Run("reaches 100 and is monotonically non-decreasing", func(t *testing.T) {
		jobID := "job-progress"
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/" + jobID) })

		chunkDir := t.TempDir()
		chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
		chunk1 := filepath.Join(chunkDir, "chunk-1.mp4")
		makeVideoChunk(t, chunk0, 3)
		makeVideoChunk(t, chunk1, 3)

		var pcts []int
		onProgress := func(pct int) { pcts = append(pcts, pct) }

		_, err := CombineChunks(jobID, map[int]string{0: chunk0, 1: chunk1}, onProgress)
		require.NoError(t, err)

		require.NotEmpty(t, pcts, "onProgress should have been called at least once")
		assert.True(t, sort.IntsAreSorted(pcts), "percents must be monotonically non-decreasing, got %v", pcts)
		assert.Equal(t, 100, pcts[len(pcts)-1], "final call must be exactly 100")
	})

	t.Run("nil onProgress does not panic and doesn't block on ffmpeg's progress pipe", func(t *testing.T) {
		jobID := "job-progress-nil"
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/" + jobID) })

		chunkDir := t.TempDir()
		chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
		makeVideoChunk(t, chunk0, 1)

		_, err := CombineChunks(jobID, map[int]string{0: chunk0}, nil)

		require.NoError(t, err)
	})
}
