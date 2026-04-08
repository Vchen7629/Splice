//go:build unit

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCombineChunksErrors(t *testing.T) {
	t.Run("MkdirAll failure returns wrapped error", func(t *testing.T) {
		// Block /tmp/jobs/job-1 by pre-creating it as a file.
		blockingFile := "/tmp/jobs/job-1"
		require.NoError(t, os.MkdirAll(filepath.Dir(blockingFile), 0755))
		require.NoError(t, os.WriteFile(blockingFile, []byte{}, 0644))
		t.Cleanup(func() { os.Remove(blockingFile) })

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"})

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

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "write manifest error")
	})

	t.Run("ffmpeg failure returns wrapped error", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-1") })

		_, err := CombineChunks("job-1", map[int]string{0: "/nonexistent/chunk.mp4"})

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

		_, err := CombineChunks("job-1", chunks)
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

		_, _ = CombineChunks("my-job", map[int]string{0: "/fake/chunk.mp4"})

		_, err := os.Stat("/tmp/jobs/my-job/manifest.txt")
		assert.NoError(t, err)
	})

	t.Run("empty chunks map writes an empty manifest", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-empty") })

		_, err := CombineChunks("job-empty", map[int]string{})
		require.Error(t, err) // ffmpeg fails with no inputs — expected

		raw, readErr := os.ReadFile("/tmp/jobs/job-empty/manifest.txt")
		require.NoError(t, readErr)
		assert.Empty(t, strings.TrimSpace(string(raw)))
	})

	t.Run("single chunk produces one file entry", func(t *testing.T) {
		t.Cleanup(func() { os.RemoveAll("/tmp/jobs/job-1") })

		_, _ = CombineChunks("job-1", map[int]string{0: "/fake/only.mp4"})

		raw, err := os.ReadFile("/tmp/jobs/job-1/manifest.txt")
		require.NoError(t, err)
		assert.Equal(t, "file '/fake/only.mp4'\n", string(raw))
	})
}
