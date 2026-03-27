//go:build integration

// fixtures and mocks
package test

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Generates a minimal black H.264 video of the given duration (seconds)
// at path using ffmpeg's lavfi source.
func MakeVideoChunk(t *testing.T, path string, seconds int) {
	t.Helper()
	cmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=320x240:d=%d", seconds),
		"-c:v", "libx264",
		"-t", strconv.Itoa(seconds),
		"-y",
		path,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to generate test video chunk: %s", out)
}

// Returns the duration of a video file in seconds using ffprobe.
func VideoDuration(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	require.NoError(t, err, "ffprobe failed on %s", path)
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	require.NoError(t, err)
	return d
}