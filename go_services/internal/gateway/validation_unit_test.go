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

	t.Run("accepts a valid mp4", func(t *testing.T) {
		path, size := makeVideo(t, "ok.mp4", small)

		assert.NoError(t, validateVideo(context.Background(), path, size))
	})

	tt := []struct{ name, file, src, wantReason string }{
		{"audio only", "audio.mp4", "sine=d=1", "no video stream"},
		{"unsupported container", "ok.flv", small, "unsupported video format"},
		{"resolution too large", "big.mp4", "color=c=black:s=8200x4700:r=1:d=1", "video resolution too large"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			path, size := makeVideo(t, tc.file, tc.src)

			assertRejected(t, validateVideo(context.Background(), path, size), tc.wantReason)
		})
	}

	t.Run("rejects empty file", func(t *testing.T) {
		assertRejected(t, validateVideo(context.Background(), "unused", 0), "empty file")
	})

	t.Run("rejects bytes that are not a video", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junk.mp4")
		require.NoError(t, os.WriteFile(path, []byte("not a video"), 0o600))

		assertRejected(t, validateVideo(context.Background(), path, 11), "unreadable video")
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
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUploadFields(tc.filename, tc.targetRes, tc.sourceRes, tc.processType)

			assert.Error(t, err)
			assert.Equal(t, tc.name, err.Error())
		})
	}

	t.Run("valid fields returns no error", func(t *testing.T) {
		assert.NoError(t, validateUploadFields("somefilename", "1080p", "1080p", "Transcode"))
	})
}
