//go:build unit

package converter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// returns each stream in the file as "codec,type" e.g. "h264,video"
func probeStreams(t *testing.T, path string) []string {
	t.Helper()
	out, err := exec.Command(
		"ffprobe", "-v", "error", "-show_entries", "stream=codec_name,codec_type", "-of", "csv=p=0", path,
	).Output()
	require.NoError(t, err, "ffprobe failed on %s", path)
	return strings.Fields(string(out))
}

func TestConvertVideo(t *testing.T) {
	audioTestVid := makeTestClip(t, true)
	noAudioTestVid := makeTestClip(t, false)

	formatTests := []struct {
		format      string
		wantStreams []string
	}{
		{"mp4", []string{"h264,video", "aac,audio"}},
		{"mov", []string{"h264,video", "aac,audio"}},
		{"mkv", []string{"h264,video", "vorbis,audio"}},
		{"webm", []string{"vp9,video", "opus,audio"}},
		{"gif", []string{"gif,video"}},
	}

	for _, tc := range formatTests {
		t.Run("converts to "+tc.format, func(t *testing.T) {
			jobID := "job-format-" + tc.format
			t.Cleanup(func() { os.RemoveAll("/tmp/convert/" + jobID) })

			outputPath, err := convertVideo(audioTestVid, handler.VideoJobMessage{JobID: jobID, TargetFormat: tc.format})

			require.NoError(t, err)
			assert.Equal(t, "/tmp/convert/"+jobID+"/output."+tc.format, outputPath)
			assert.Equal(t, tc.wantStreams, probeStreams(t, outputPath))
		})
	}

	t.Run("source without audio converts video only", func(t *testing.T) {
		jobID := "job-no-audio"
		t.Cleanup(func() { os.RemoveAll("/tmp/convert/" + jobID) })

		outputPath, err := convertVideo(noAudioTestVid, handler.VideoJobMessage{JobID: jobID, TargetFormat: "mp4"})

		require.NoError(t, err)
		assert.Equal(t, []string{"h264,video"}, probeStreams(t, outputPath))
	})

	for _, format := range []string{"", "mp3", "MP4", "../x"} {
		t.Run("rejects unsupported format "+format, func(t *testing.T) {
			jobID := "job-unsupported"
			t.Cleanup(func() { os.RemoveAll("/tmp/convert/" + jobID) })

			outputPath, err := convertVideo(audioTestVid, handler.VideoJobMessage{JobID: jobID, TargetFormat: format})

			require.ErrorContains(t, err, "unsupported target format")
			assert.Empty(t, outputPath)
			assert.NoDirExists(t, "/tmp/convert/"+jobID, "output dir should not be created before format is validated")
		})
	}

	t.Run("mkdir failure returns create output dir error", func(t *testing.T) {
		jobID := "job-blocked"
		require.NoError(t, os.MkdirAll("/tmp/convert", 0755))
		require.NoError(t, os.WriteFile("/tmp/convert/"+jobID, []byte("blocker"), 0644))
		t.Cleanup(func() { os.Remove("/tmp/convert/" + jobID) })

		outputPath, err := convertVideo(audioTestVid, handler.VideoJobMessage{JobID: jobID, TargetFormat: "mp4"})

		require.ErrorContains(t, err, "create output convert dir error")
		assert.Empty(t, outputPath)
	})

	t.Run("ffmpeg timeout returns context deadline exceeded", func(t *testing.T) {
		orig := ffmpegTimeout
		ffmpegTimeout = time.Microsecond
		t.Cleanup(func() {
			ffmpegTimeout = orig
			os.RemoveAll("/tmp/convert/job-timeout")
		})

		_, err := convertVideo(audioTestVid, handler.VideoJobMessage{JobID: "job-timeout", TargetFormat: "mp4"})

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out")
	})

	notVideo := filepath.Join(t.TempDir(), "fake.mp4")
	require.NoError(t, os.WriteFile(notVideo, []byte("not a video"), 0644))

	ffmpegErrorTests := []struct {
		name string
		src  string
	}{
		{"missing input file returns ffmpeg error", "/nonexistent/video.mp4"},
		{"non-video input returns ffmpeg error", notVideo},
	}

	for _, tc := range ffmpegErrorTests {
		t.Run(tc.name, func(t *testing.T) {
			jobID := "job-ffmpeg-error"
			t.Cleanup(func() { os.RemoveAll("/tmp/convert/" + jobID) })

			_, err := convertVideo(tc.src, handler.VideoJobMessage{JobID: jobID, TargetFormat: "mp4"})

			require.ErrorContains(t, err, "ffmpeg error")
		})
	}
}
