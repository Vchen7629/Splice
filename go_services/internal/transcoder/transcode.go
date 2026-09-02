package transcoder

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
func TranscodeVideo(filePath, target_resolution, chunkName string, logger *slog.Logger) (string, error) {
	outDir := "/tmp/temp-processed-" + chunkName
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output dir error: %w", err)
	}
	// output must always be mp4 since always re-encoded to h264
	filename := filepath.Base(filePath)
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	outputPath := filepath.Join(outDir, stem+".mp4")
	height := strings.TrimSuffix(target_resolution, "p")

	cmd := exec.Command(
		"ffmpeg",
		"-i", filePath,
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
