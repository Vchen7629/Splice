package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// stitches transcoded video chunks for a job into a single output video file
// chunks is a map of chunk index -> output path, sorted by index
func CombineChunks(jobID string, chunks map[int]string) (string, error) {
	outDir := filepath.Join("/tmp/jobs", jobID)

	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output dir error: %w", err)
	}

	indices := make([]int, 0, len(chunks))
	for i := range chunks {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	var sb strings.Builder
	for _, i := range indices {
		fmt.Fprintf(&sb, "file '%s'\n", chunks[i])
	}

	// manifest path contains a txt file listing all the inputs (video paths) to combine
	manifestPath := filepath.Join(outDir, "manifest.txt")
	err = os.WriteFile(manifestPath, []byte(sb.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("write manifest error: %w", err)
	}

	outputPath := filepath.Join(outDir, "output.mp4")
	cmd := exec.Command(
		"ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", manifestPath,
		"-c", "copy",
		"-y",
		outputPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg concat error: %w\n%s", err, out)
	}

	return outputPath, nil
}
