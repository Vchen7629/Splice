package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/pipeline"
)

var ffmpegTimeout = 30 * time.Minute // TODO: in the future scale timeout with input video size via ffprobe

// convert the source video to target format (mp4/mov/mkv/webm/gif) using ffmpeg and return the converted video
// local filepath so it can be uploaded to seaweedfs storage so the user can download it later
func convertVideo(filePath string, payload pipeline.VideoJobMessage) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	baseArgs := []string{"-protocol_whitelist", "file,pipe", "-i", filePath, "-map", "0:v:0"}

	var formatArgs []string
	switch payload.TargetFormat {
	case "mp4", "mov", "mkv":
		formatArgs = []string{"-map", "0:a:0?", "-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2", "-pix_fmt", "yuv420p"}
	case "webm":
		formatArgs = []string{"-map", "0:a:0?", "-crf", "32", "-b:v", "0"}
	case "gif":
		formatArgs = []string{"-vf", "fps=15,scale=400:-1"}
	default:
		return "", fmt.Errorf("unsupported target format: %s", payload.TargetFormat)
	}

	outDir := "/tmp/convert/" + payload.JobID
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output convert dir error: %w", err)
	}

	outputPath := filepath.Join(outDir, "output."+payload.TargetFormat)

	convertArgs := append(baseArgs, formatArgs...)
	convertArgs = append(convertArgs, "-y", outputPath)

	out, err := exec.CommandContext(ctx, "ffmpeg", convertArgs...).CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("ffmpeg timed out after %s: %w", ffmpegTimeout, ctxErr)
		}
		return "", fmt.Errorf("ffmpeg error: %w\n%s", err, out)
	}

	return outputPath, nil
}
