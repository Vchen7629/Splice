package recombiner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	recombineChunkTimeout = 15 * time.Minute // TODO: in the future update this to scale with video size via ffprobe
	probeDurationTimeout  = 15 * time.Second
)

// stitches transcoded video chunks for a job into a single output video file
// chunks is a map of chunk index -> output path, sorted by index
func CombineChunks(jobID string, chunks map[int]string, onProgress func(pct int)) (string, error) {
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

	// duration only used to scale progress pcts. chunks that fail to probe just contribute 0
	// rather than failing entire combine
	var totalDurationSeconds float64
	for _, i := range indices {
		duration, probeErr := probeDurationSeconds(chunks[i])
		if probeErr == nil {
			totalDurationSeconds += duration
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), recombineChunkTimeout)
	defer cancel()

	outputPath := filepath.Join(outDir, "output.mp4")
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", manifestPath,
		"-c", "copy",
		"-progress", "pipe:1",
		"-y",
		outputPath,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("ffmpeg stdout pipe error: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	wrapFfmpegErr := func(err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("ffmpeg concat timed out after %s: %w", recombineChunkTimeout, ctxErr)
		}
		return fmt.Errorf("ffmpeg concat error: %w\n%s", err, stderr.String())
	}

	err = cmd.Start()
	if err != nil {
		return "", wrapFfmpegErr(err)
	}

	parseProgress(bufio.NewScanner(stdout), totalDurationSeconds, onProgress)

	err = cmd.Wait()
	if err != nil {
		return "", wrapFfmpegErr(err)
	}

	if onProgress != nil {
		onProgress(100)
	}

	return outputPath, nil
}

// returns a video file's duration in seconds via ffprobe
func probeDurationSeconds(filePath string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeDurationTimeout)
	defer cancel()

	out, err := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		filePath,
	).Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, fmt.Errorf("ffprobe timed out after %s: %w", probeDurationTimeout, ctxErr)
		}
		return 0, fmt.Errorf("ffprobe duration error: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse ffprobe duration output %q: %w", out, err)
	}

	return duration, nil
}

// parseProgress reads ffmpeg's `-progress pipe:1` key=value stream, draining it until
// ffmpeg exits, and reports percent complete (capped below 100
func parseProgress(stdout *bufio.Scanner, totalDurationSeconds float64, onProgress func(pct int)) {
	lastPct := -1
	for stdout.Scan() {
		outTimeMs, ok := strings.CutPrefix(stdout.Text(), "out_time_ms=")
		if !ok || onProgress == nil || totalDurationSeconds <= 0 {
			continue
		}

		// ffmpeg's out_time_ms field is actually microseconds
		outTimeUs, err := strconv.ParseInt(outTimeMs, 10, 64)
		if err != nil {
			continue
		}

		pct := min(int(float64(outTimeUs)/1_000_000.0/totalDurationSeconds*100), 99)
		if pct != lastPct {
			lastPct = pct
			onProgress(pct)
		}
	}
}
