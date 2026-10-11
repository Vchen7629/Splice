//go:build unit

package gateway

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generates a tiny clip with ffmpeg into temp dir, skips test if ffmpeg is missing
func makeVideo(t *testing.T, name, lavfiSrc string) (string, int64) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), name)
	out, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", lavfiSrc, path).CombinedOutput()
	require.NoError(t, err, string(out))
	info, err := os.Stat(path)
	require.NoError(t, err)
	return path, info.Size()
}

func assertRejected(t *testing.T, err error, wantReason string) {
	t.Helper()
	var rejected *rejectError
	require.True(t, errors.As(err, &rejected), "want *rejectError, got %v", err)
	assert.Equal(t, wantReason, rejected.reason)
}

func TestValidateJobID(t *testing.T) {
	t.Run("Returns error if jobID is empty", func(t *testing.T) {
		err := validateJobID("")

		assert.Error(t, err)
	})

	t.Run("Returns error if jobID is non uuid", func(t *testing.T) {
		err := validateJobID("some uuid")

		assert.Error(t, err)
	})
}

func TestValidateVideo(t *testing.T) {
	const small = "color=c=black:s=64x64:r=1:d=1"
	const smallRes = "64p" // small is 64x64

	t.Run("accepts a valid mp4", func(t *testing.T) {
		path, size := makeVideo(t, "ok.mp4", small)

		assert.NoError(t, validateVideo(context.Background(), path, smallRes, "Transcode", size))
	})

	t.Run("Doesnt error for Non resolution service (Denoise) that supplies non matching res", func(t *testing.T) {
		path, size := makeVideo(t, "ok.mp4", small)

		assert.NoError(t, validateVideo(context.Background(), path, "999p", "Denoise", size))
	})

	tt := []struct{ name, file, src, videoRes, wantReason string }{
		{"audio only", "audio.mp4", "sine=d=1", smallRes, "no video stream"},
		{"unsupported container", "ok.flv", small, smallRes, "unsupported video format"},
		{"resolution too large", "big.mp4", "color=c=black:s=8200x4700:r=1:d=1", smallRes, "video resolution too large"},
		{"rejects source resolution that doesnt match", "ok.mp4", small, "200p", "source_resolution doesn't match actual video resolution"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			path, size := makeVideo(t, tc.file, tc.src)

			assertRejected(t, validateVideo(context.Background(), path, tc.videoRes, "Transcode", size), tc.wantReason)
		})
	}

	t.Run("rejects empty file", func(t *testing.T) {
		assertRejected(t, validateVideo(context.Background(), "unused", small, "Transcode", 0), "empty file")
	})

	t.Run("rejects bytes that are not a video", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junk.mp4")
		require.NoError(t, os.WriteFile(path, []byte("not a video"), 0o600))

		assertRejected(t, validateVideo(context.Background(), path, small, "Transcode", 11), "unreadable video")
	})
}

func TestValidateUploadFields(t *testing.T) {
	tt := []struct {
		name, targetRes, sourceRes, processType, filename string
	}{
		{"missing target_resolution field", "", "1080p", "Transcode", "somefile"},
		{"missing source_resolution field", "1080p", "", "Transcode", "somefile"},
		{"missing process_type field", "1080p", "1080p", "", "somefile"},
		{"invalid process_type field", "1080p", "1080p", "IDK", "somefile"},
		{"invalid video filename", "1080p", "1080p", "Transcode", "../etc/passwd"}, // path traversal
		{"invalid video filename", "1080p", "1080p", "Transcode", ""},
		{"uploaded file target res not allowed", "9999p", "1080p", "Transcode", "somefile"},
		{"uploaded file source res not allowed", "1080p", "92p", "Transcode", "somefile"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUploadFields(tc.filename, tc.targetRes, tc.sourceRes, tc.processType)

			assert.Error(t, err)
			assert.Equal(t, tc.name, err.Error())
		})
	}

	t.Run("denoise and convert ignore resolution fields", func(t *testing.T) {
		assert.NoError(t, validateUploadFields("somefilename", "", "", "Denoise"))
		assert.NoError(t, validateUploadFields("somefilename", "", "", "Convert"))
		assert.NoError(t, validateUploadFields("somefilename", "1080p", "92p", "Denoise")) // ignored, not validated
	})

	t.Run("valid fields returns no error", func(t *testing.T) {
		assert.NoError(t, validateUploadFields("somefilename", "1080p", "1080p", "Transcode"))
	})
}
