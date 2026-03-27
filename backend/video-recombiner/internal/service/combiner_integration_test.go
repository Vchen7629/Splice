//go:build integration

package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCombineChunksProducesOutputFile(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
	chunk1 := filepath.Join(chunkDir, "chunk-1.mp4")
	test.MakeVideoChunk(t, chunk0, 1)
	test.MakeVideoChunk(t, chunk1, 1)

	outputPath, err := service.CombineChunks("job-full", map[int]string{0: chunk0, 1: chunk1}, outputDir)

	require.NoError(t, err)
	info, statErr := os.Stat(outputPath)
	require.NoError(t, statErr, "output.mp4 should exist")
	assert.Greater(t, info.Size(), int64(0), "output.mp4 should be non-empty")
}

func TestOutputPathIsCorrectlyFormed(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	chunk0 := filepath.Join(chunkDir, "chunk-0.mp4")
	test.MakeVideoChunk(t, chunk0, 1)

	outputPath, err := service.CombineChunks("my-job", map[int]string{0: chunk0}, outputDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(outputDir, "jobs", "my-job", "output.mp4"), outputPath)
}

// TestSingleChunkCombineSucceeds verifies that a single-chunk job completes without error.
func TestSingleChunkCombineSucceeds(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	chunk0 := filepath.Join(chunkDir, "only-chunk.mp4")
	test.MakeVideoChunk(t, chunk0, 1)

	outputPath, err := service.CombineChunks("job-single", map[int]string{0: chunk0}, outputDir)

	require.NoError(t, err)
	_, statErr := os.Stat(outputPath)
	assert.NoError(t, statErr)
}

func TestAllChunksIncludedInOutput(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	// chunks have durations 1s, 2s, 3s — total 6s
	durations := map[int]int{0: 1, 1: 2, 2: 3}
	chunks := make(map[int]string, len(durations))
	var totalSeconds float64
	for idx, d := range durations {
		p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
		test.MakeVideoChunk(t, p, d)
		chunks[idx] = p
		totalSeconds += float64(d)
	}

	outputPath, err := service.CombineChunks("job-order", chunks, outputDir)

	require.NoError(t, err)
	got := test.VideoDuration(t, outputPath)
	assert.InDelta(t, totalSeconds, got, 0.1, "output duration should equal sum of all chunk durations")
}

func TestNonContiguousIndicesAllIncluded(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	indices := map[int]int{0: 1, 5: 2, 10: 1} // index -> duration
	chunks := make(map[int]string, len(indices))
	var totalSeconds float64
	for idx, d := range indices {
		p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
		test.MakeVideoChunk(t, p, d)
		chunks[idx] = p
		totalSeconds += float64(d)
	}

	outputPath, err := service.CombineChunks("job-gaps", chunks, outputDir)

	require.NoError(t, err)
	got := test.VideoDuration(t, outputPath)
	assert.InDelta(t, totalSeconds, got, 0.1)
}

// Verifies that a job whose chunk indices start above 0
// (e.g. a partial re-deliver starting at index 3) combines correctly.
func TestNonZeroBasedIndicesAllIncluded(t *testing.T) {
	chunkDir := t.TempDir()
	outputDir := t.TempDir()

	indices := map[int]int{3: 1, 4: 2, 5: 1}
	chunks := make(map[int]string, len(indices))
	var totalSeconds float64
	for idx, d := range indices {
		p := filepath.Join(chunkDir, fmt.Sprintf("chunk-%d.mp4", idx))
		test.MakeVideoChunk(t, p, d)
		chunks[idx] = p
		totalSeconds += float64(d)
	}

	outputPath, err := service.CombineChunks("job-nonzero", chunks, outputDir)

	require.NoError(t, err)
	got := test.VideoDuration(t, outputPath)
	assert.InDelta(t, totalSeconds, got, 0.1)
}
