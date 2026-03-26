package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// transcode by upscaling/downscaling the input video chunk to the specified value and output and return
// the resulting video chunk. Uses lanczos algorithm is used for upscaling for now since AI super resolution
// algo requires libtensorflow + model file and is scoped to a later phase
func TranscodeVideo(payload VideoChunkMessage, outputDir string, logger *slog.Logger) (string, error) {
	outDir := filepath.Join(outputDir, "jobs", payload.JobID, "transcoded")
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output dir error: %w", err)
	}

	outputPath := filepath.Join(outDir, fmt.Sprintf("chunk_%03d.mp4", payload.ChunkIndex))
	height := strings.TrimSuffix(payload.TargetResolution, "p")

	cmd := exec.Command(
		"ffmpeg",
		"-i", payload.StoragePath,
		"-vf", fmt.Sprintf("scale=-2:%s:flags=lanczos", height),
		"-c:v", "libx264",
		"-c:a", "copy",
		"-y",
		outputPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %w\n%s", err, out)
	}

	return outputPath, nil
}
