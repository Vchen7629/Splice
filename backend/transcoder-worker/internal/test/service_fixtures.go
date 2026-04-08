package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"transcoder-worker/internal/service"

	"github.com/stretchr/testify/require"
)

func VideoHeight(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=height",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	require.NoError(t, err, "ffprobe failed on %s", path)
	height, err := strconv.Atoi(strings.TrimSpace(string(out)))
	require.NoError(t, err, "unexpected ffprobe output: %q", string(out))
	return height
}

func BasePayload() service.VideoChunkMessage {
	return service.VideoChunkMessage{
		JobID:            "job-abc",
		ChunkIndex:       0,
		StorageURL:       "/some/input.mp4",
		TargetResolution: "720p",
	}
}

func BlockDirCreation(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jobs"), []byte("blocker"), 0644))
}
