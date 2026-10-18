package chunkworker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"
)

var transcodeVideoFn = transcodeVideo

// process a video chunk from nats msg. Includes Updating the job status to be chunk-worker stage -> fetching the chunk
// -> transcoding it. returns the outputPath and the error
func processChunk(jobMilestoneKV jetstream.KeyValue, payload VideoChunkMessage, logger *slog.Logger) (string, error) {
	err := sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.JobStatus{State: "PROCESSING", Stage: "chunk-worker"})
	if err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		return "", err
	}

	chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)

	filePath, err := storage.GetVideoChunk(payload.StorageURL, chunkName)
	if err != nil {
		logger.Error("error fetching unprocessed video chunk", "job_id", payload.JobID, "err", err)
		return "", err
	}

	if payload.Operation == "transcode" {
		outputPath, err := transcodeVideoFn(filePath, payload.TargetResolution, chunkName, logger)
		if err != nil {
			logger.Error("error transcoding chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			return "", err
		}
		return outputPath, nil
	}

	outputPath, err := denoiseVideo(filePath, chunkName, logger)
	if err != nil {
		logger.Error("error denoising chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return "", err
	}

	return outputPath, nil
}

var ffmpegTimeout = 2 * time.Hour // TODO: in the future scale timeout with input video size via ffprobe

// transcode by resizing the input video chunk to the specified value and output and return resulting video chunk.
func transcodeVideo(filePath, target_resolution, chunkName string, logger *slog.Logger) (string, error) {
	outDir := "/tmp/transcode/temp-processed-" + chunkName
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output transcode dir error: %w", err)
	}
	// output must always be mp4 since always re-encoded to h264
	filename := filepath.Base(filePath)
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	outputPath := filepath.Join(outDir, stem+".mp4")
	height := strings.TrimSuffix(target_resolution, "p")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	out, err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-protocol_whitelist", "file,pipe", "-i", filePath,
		"-vf", fmt.Sprintf("scale=-2:%s:flags=lanczos", height),
		"-c:v", "libx264",
		"-c:a", "copy",
		"-y",
		outputPath,
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
// NOTE: in the future could give user option of different quality since there are multiple denoise flags
func denoiseVideo(filePath, chunkName string, logger *slog.Logger) (string, error) {
	outDir := "/tmp/denoise/temp-processed-" + chunkName
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		return "", fmt.Errorf("create output denoise dir error: %w", err)
	}
	filename := filepath.Base(filePath)
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	outputPath := filepath.Join(outDir, stem+".mp4")

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	out, err := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-protocol_whitelist", "file,pipe", "-i", filePath,
		"-vf", "hqdn3d=luma_spatial=4.0:chroma_spatial=3.0:luma_tmp=6.0:chroma_tmp=4.5",
		"-c:v", "libx264",
		"-crf", "18",
		outputPath,
	).CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("ffmpeg timed out after %s: %w", ffmpegTimeout, ctxErr)
		}
		return "", fmt.Errorf("ffmpeg error: %w\n%s", err, out)
	}

	return outputPath, nil
}
