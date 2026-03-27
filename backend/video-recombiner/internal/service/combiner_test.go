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

// triggers MkdirAll failure by passing a regular file as a path component
func TestMkdirAllFailsReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte{}, 0644))

	_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, blockingFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create output dir error")
}

// pre-create the output dir as read-only so WriteFile cannot write manifest.txt inside it.
func TestWriteManifestFailsReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "jobs", "job-1")
	require.NoError(t, os.MkdirAll(outDir, 0755))
	require.NoError(t, os.Chmod(outDir, 0555))
	t.Cleanup(func() { os.Chmod(outDir, 0755) }) // allow cleanup

	_, err := CombineChunks("job-1", map[int]string{0: "chunk.mp4"}, tmpDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write manifest error")
}

// Verifies that the manifest lists files in ascending index order regardless of how 
// the map was constructed. ffmpeg will fail on the fake paths but the manifest is
// written before ffmpeg runs.
func TestManifestIsSortedByChunkIndex(t *testing.T) {
	tmpDir := t.TempDir()
	chunks := map[int]string{
		2: "/fake/c.mp4",
		0: "/fake/a.mp4",
		1: "/fake/b.mp4",
	}

	_, err := CombineChunks("job-1", chunks, tmpDir)

	// ffmpeg will fail on fake paths — that's expected
	require.Error(t, err)

	manifestPath := filepath.Join(tmpDir, "jobs", "job-1", "manifest.txt")
	raw, readErr := os.ReadFile(manifestPath)
	require.NoError(t, readErr, "manifest.txt should exist even when ffmpeg fails")

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "file '/fake/a.mp4'", lines[0])
	assert.Equal(t, "file '/fake/b.mp4'", lines[1])
	assert.Equal(t, "file '/fake/c.mp4'", lines[2])
}

// Verifies the manifest is written to outputDir/jobs/<jobID>/manifest.txt.
func TestManifestPathIsInsideOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	_, _ = CombineChunks("my-job", map[int]string{0: "/fake/chunk.mp4"}, tmpDir)

	expected := filepath.Join(tmpDir, "jobs", "my-job", "manifest.txt")
	_, err := os.Stat(expected)
	assert.NoError(t, err, "manifest.txt should be at outputDir/jobs/<jobID>/manifest.txt")
}

// Verifies that when ffmpeg exits non-zero the error message includes "ffmpeg concat error".
func TestFFmpegFailureWrappedInError(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := CombineChunks("job-1", map[int]string{0: "/nonexistent/chunk.mp4"}, tmpDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg concat error")
}

// Verifies that an empty chunk map writes an empty manifest and returns an ffmpeg error 
// (concat with no inputs fails).
func TestEmptyChunksMapProducesEmptyManifest(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := CombineChunks("job-empty", map[int]string{}, tmpDir)

	manifestPath := filepath.Join(tmpDir, "jobs", "job-empty", "manifest.txt")
	raw, readErr := os.ReadFile(manifestPath)
	require.NoError(t, readErr, "manifest.txt should be written even for empty chunk map")
	assert.Empty(t, strings.TrimSpace(string(raw)))
	require.Error(t, err)
}

// Verifies manifest content for a single chunk.
func TestSingleChunkManifestHasOneEntry(t *testing.T) {
	tmpDir := t.TempDir()

	_, _ = CombineChunks("job-1", map[int]string{0: "/fake/only.mp4"}, tmpDir)

	manifestPath := filepath.Join(tmpDir, "jobs", "job-1", "manifest.txt")
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, "file '/fake/only.mp4'\n", string(raw))
}
