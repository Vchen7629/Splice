//go:build unit

package gateway

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// puts a fake ffprobe first on PATH that prints stdout and exits with exitCode,
// returns the file the fake ffprobe writes the args it was called with to
func fakeFfprobe(t *testing.T, stdout string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	outFile := filepath.Join(dir, "out")
	require.NoError(t, os.WriteFile(outFile, []byte(stdout), 0o600))

	script := "#!/bin/sh\necho \"$@\" > '" + argsFile + "'\ncat '" + outFile + "'\nexit " + string(rune('0'+exitCode)) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ffprobe"), []byte(script), 0o700)) //nolint:gosec
	t.Setenv("PATH", dir+":/usr/bin:/bin")                                                 // fake first, so cat still resolves

	return argsFile
}

// generates a tiny clip with ffmpeg into temp dir, extraArgs go before the output path,
// skips test if ffmpeg is missing
func makeVideo(t *testing.T, name, lavfiSrc string, extraArgs ...string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), name)
	args := append([]string{"-v", "error", "-y", "-f", "lavfi", "-i", lavfiSrc}, extraArgs...)
	out, err := exec.Command("ffmpeg", append(args, path)...).CombinedOutput()
	require.NoError(t, err, string(out))

	return path
}

func TestProbeVideo(t *testing.T) {
	const validOut = `{"streams":[{"width":1920,"height":1080,"nb_read_packets":"240"}],` +
		`"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"}}`

	t.Run("parses format, width, height and packets", func(t *testing.T) {
		fakeFfprobe(t, validOut, 0)

		info, err := probeVideo(context.Background(), "video.mp4")

		require.NoError(t, err)
		assert.Equal(t, videoInfo{format: "mov,mp4,m4a,3gp,3g2,mj2", width: 1920, height: 1080, packets: 240}, info)
	})

	t.Run("restricts ffprobe to the file protocol and probes the given path", func(t *testing.T) {
		argsFile := fakeFfprobe(t, validOut, 0)

		_, err := probeVideo(context.Background(), "some/video.mp4")
		require.NoError(t, err)

		args, err := os.ReadFile(argsFile)
		require.NoError(t, err)
		assert.Contains(t, string(args), "-protocol_whitelist file")
		assert.Contains(t, string(args), "some/video.mp4")
	})

	t.Run("missing packet count parses as zero", func(t *testing.T) {
		fakeFfprobe(t, `{"streams":[{"width":64,"height":64}],"format":{"format_name":"avi"}}`, 0)

		info, err := probeVideo(context.Background(), "video.avi")

		require.NoError(t, err)
		assert.Equal(t, int64(0), info.packets)
	})

	rejects := []struct {
		name, stdout string
		exitCode     int
		wantReason   string
	}{
		{"no streams in output", `{"streams":[],"format":{"format_name":"mp4"}}`, 0, "no video stream"},
		{"output is not json", "not json", 0, "no video stream"},
		{"empty output", "", 0, "no video stream"},
		{"ffprobe exits non zero", "", 1, "unreadable video"},
	}
	for _, tc := range rejects {
		t.Run("rejects when "+tc.name, func(t *testing.T) {
			fakeFfprobe(t, tc.stdout, tc.exitCode)

			_, err := probeVideo(context.Background(), "video.mp4")

			assertRejected(t, err, tc.wantReason)
		})
	}

	t.Run("returns a plain error when the context is cancelled", func(t *testing.T) {
		fakeFfprobe(t, validOut, 0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := probeVideo(ctx, "video.mp4")

		require.Error(t, err)
		assert.NotErrorAs(t, err, new(*rejectError))
	})

	const small = "color=c=black:s=64x48:r=1:d=1" // 64x48, 1 frame

	t.Run("returns format, dimensions and packet count of a valid mp4", func(t *testing.T) {
		path := makeVideo(t, "ok.mp4", small)

		info, err := probeVideo(context.Background(), path)

		require.NoError(t, err)
		assert.Contains(t, info.format, "mp4")
		assert.Equal(t, 64, info.width)
		assert.Equal(t, 48, info.height)
		assert.Equal(t, int64(1), info.packets)
	})

	t.Run("reports the container so validateVideo can reject it", func(t *testing.T) {
		path := makeVideo(t, "ok.flv", small)

		info, err := probeVideo(context.Background(), path)

		require.NoError(t, err)
		assert.Equal(t, "flv", info.format)
	})

	t.Run("rejects audio only files", func(t *testing.T) {
		path := makeVideo(t, "audio.mp4", "sine=d=1")

		_, err := probeVideo(context.Background(), path)

		assertRejected(t, err, "no video stream")
	})

	t.Run("rejects bytes that are not a video", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junk.mp4")
		require.NoError(t, os.WriteFile(path, []byte("not a video"), 0o600))

		_, err := probeVideo(context.Background(), path)

		assertRejected(t, err, "unreadable video")
	})
}
