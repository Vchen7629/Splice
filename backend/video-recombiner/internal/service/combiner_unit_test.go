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
		// Pass a regular file as a path component to trigger MkdirAll failure.
		tmpDir := t.TempDir()
		blockingFile := filepath.Join(tmpDir, "not-a-dir")
		require.NoError(t, os.WriteFile(blockingFile, []byte{}, 0644))

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, blockingFile)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "create output dir error")
	})

	t.Run("manifest write failure returns wrapped error", func(t *testing.T) {
		// Pre-create the output dir as read-only so WriteFile cannot write manifest.txt.
		tmpDir := t.TempDir()
		outDir := filepath.Join(tmpDir, "jobs", "job-1")
		require.NoError(t, os.MkdirAll(outDir, 0755))
		require.NoError(t, os.Chmod(outDir, 0555))
		t.Cleanup(func() { os.Chmod(outDir, 0755) })

		_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, tmpDir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "write manifest error")
	})

	t.Run("ffmpeg failure returns wrapped error", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := CombineChunks("job-1", map[int]string{0: "/nonexistent/chunk.mp4"}, tmpDir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffmpeg concat error")
	})
}

func TestManifest(t *testing.T) {
	t.Run("is sorted by chunk index regardless of map order", func(t *testing.T) {
		tmpDir := t.TempDir()
		chunks := map[int]string{
			2: "/fake/c.mp4",
			0: "/fake/a.mp4",
			1: "/fake/b.mp4",
		}

		_, err := CombineChunks("job-1", chunks, tmpDir)
		require.Error(t, err) // ffmpeg fails on fake paths — expected

		raw, readErr := os.ReadFile(filepath.Join(tmpDir, "jobs", "job-1", "manifest.txt"))
		require.NoError(t, readErr, "manifest.txt should exist even when ffmpeg fails")

		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		require.Len(t, lines, 3)
		assert.Equal(t, "file '/fake/a.mp4'", lines[0])
		assert.Equal(t, "file '/fake/b.mp4'", lines[1])
		assert.Equal(t, "file '/fake/c.mp4'", lines[2])
	})

	t.Run("is written to outputDir/jobs/{jobID}/manifest.txt", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, _ = CombineChunks("my-job", map[int]string{0: "/fake/chunk.mp4"}, tmpDir)

		_, err := os.Stat(filepath.Join(tmpDir, "jobs", "my-job", "manifest.txt"))
		assert.NoError(t, err)
	})

	t.Run("empty chunks map writes an empty manifest", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := CombineChunks("job-empty", map[int]string{}, tmpDir)
		require.Error(t, err) // ffmpeg fails with no inputs — expected

		raw, readErr := os.ReadFile(filepath.Join(tmpDir, "jobs", "job-empty", "manifest.txt"))
		require.NoError(t, readErr)
		assert.Empty(t, strings.TrimSpace(string(raw)))
	})

	t.Run("single chunk produces one file entry", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, _ = CombineChunks("job-1", map[int]string{0: "/fake/only.mp4"}, tmpDir)

		raw, err := os.ReadFile(filepath.Join(tmpDir, "jobs", "job-1", "manifest.txt"))
		require.NoError(t, err)
		assert.Equal(t, "file '/fake/only.mp4'\n", string(raw))
	})
}
