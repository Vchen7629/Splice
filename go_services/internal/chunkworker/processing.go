package chunkworker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"
)

const downloadTimeout = 30 * time.Second

var transcodeVideoFn = transcodeVideo

// process a video chunk from nats msg. Fetches the chunk -> transcods it. returns the outputPath and the error
func processChunk(payload pipeline.VideoChunkMessage) (string, error) {
	chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)

	switch payload.Operation {
	case "transcode":
	case "denoise":
		switch payload.DenoiseQuality {
		case "light", "medium":
		default:
			return "", fmt.Errorf("unsupported quality selected: %s", payload.DenoiseQuality)
		}
	default:
		return "", fmt.Errorf("unsupported chunk operation: %s", payload.Operation)
	}

	if err := storage.CheckExists(payload.StorageURL, downloadTimeout); err != nil {
		return "", err
	}

	if payload.Operation == "transcode" {
		outputPath, err := transcodeVideoFn(payload.StorageURL, payload.TargetResolution, chunkName)
		if err != nil {
			return "", err
		}
		return outputPath, nil
	}

	outputPath, err := denoiseVideo(payload.StorageURL, payload.DenoiseQuality, chunkName)
	if err != nil {
		return "", err
	}

	return outputPath, nil
}

var ffmpegTimeout = 2 * time.Hour // TODO: in the future scale timeout with input video size via ffprobe

// transcode by resizing the input video chunk to the specified value and output and return resulting video chunk.
func transcodeVideo(filePath, target_resolution, chunkName string) (string, error) {
	outDir := "/tmp/transcode/temp-processed-" + chunkName
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output transcode dir error: %w", err)
	}
	// output must always be mp4 since always re-encoded to h264. named from chunkName rather than the
	// input url since the url contains the user-supplied filename, which can exceed NAME_MAX once escaped
	outputPath := filepath.Join(outDir, chunkName+".mp4")
	height := strings.TrimSuffix(target_resolution, "p")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	out, err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-reconnect", "1", "-reconnect_on_network_error", "1", "-reconnect_on_http_error", "5xx", "-reconnect_delay_max", "10",
		"-protocol_whitelist", "http,tcp", "-f", "matroska", "-i", filePath,
		"-vf", fmt.Sprintf("scale=-2:%s:flags=lanczos", height),
		"-c:v", "libx264",
		"-an", "-fps_mode", "passthrough", "-pix_fmt", "yuv420p", "-video_track_timescale", "90000",
		"-y", outputPath,
	).CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("ffmpeg timed out after %s: %w", ffmpegTimeout, ctxErr)
		}
		return "", fmt.Errorf("ffmpeg error: %w\n%s", err, out)
	}

	return outputPath, nil
}

// denoise the unprocessed video and return resulting video chunk
func denoiseVideo(filePath, denoiseQuality, chunkName string) (string, error) {
	outDir := "/tmp/denoise/temp-processed-" + chunkName
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output denoise dir error: %w", err)
	}
	outputPath := filepath.Join(outDir, chunkName+".mp4")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	var filterStrength string
	switch denoiseQuality {
	case "light":
		filterStrength = "hqdn3d=luma_spatial=4.0:chroma_spatial=3.0:luma_tmp=6.0:chroma_tmp=4.5"
	case "medium":
		filterStrength = "hqdn3d=luma_spatial=8.0:chroma_spatial=6.0:luma_tmp=12.0:chroma_tmp=9.0"
	// in the future add strong filter option that uses nlmeans (potentially with gpu version instead of on cpu)
	default:
		return "", fmt.Errorf("unsupported quality selected: %s", denoiseQuality)
	}

	out, err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-reconnect", "1", "-reconnect_on_network_error", "1", "-reconnect_on_http_error", "5xx", "-reconnect_delay_max", "10",
		"-protocol_whitelist", "http,tcp", "-f", "matroska", "-i", filePath,
		"-vf", filterStrength,
		"-c:v", "libx264",
		"-crf", "18",
		"-an", "-fps_mode", "passthrough", "-pix_fmt", "yuv420p", "-video_track_timescale", "90000",
		"-y", outputPath,
	).CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("ffmpeg timed out after %s: %w", ffmpegTimeout, ctxErr)
		}
		return "", fmt.Errorf("ffmpeg error: %w\n%s", err, out)
	}

	return outputPath, nil
}
