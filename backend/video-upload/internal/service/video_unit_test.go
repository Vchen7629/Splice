//go:build unit

package service_test

import (
	"io"
	"os"
	"testing"
	"video-upload/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckVideoResolution(t *testing.T) {
	// the mock video is a 720p video so we are just checking its correct
	t.Run("720p video", func(t *testing.T) {
		f, err := os.Open("../test/testvideo.mp4")
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })

		result, err := service.CheckVideoResolution(f)

		require.NoError(t, err)
		assert.Equal(t, "720p", result)
	})

	t.Run("file cursor is reset to zero after probing", func(t *testing.T) {
		f, err := os.Open("../test/testvideo.mp4")
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })

		_, err = service.CheckVideoResolution(f)
		require.NoError(t, err)

		pos, err := f.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pos)
	})

	t.Run("nil reader returns ffprobe error", func(t *testing.T) {
		result, err := service.CheckVideoResolution(nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ffprobe error")
		assert.Empty(t, result)
	})
}

func TestConvertStringToInt(t *testing.T) {
	tests := []struct {
		resString string
		expected  int
	}{{"720p", int(720)}, {"480p", int(480)}, {"1080P", int(1080)}}
	for _, tt := range tests {
		t.Run("Converts it properly to int", func(t *testing.T) {
			res := service.ConvertResStringToInt(tt.resString)

			assert.Equal(t, tt.expected, res)
		})
	}

	t.Run("Handles no p suffix case", func(t *testing.T) {
		res := service.ConvertResStringToInt("720")

		assert.Equal(t, int(720), res)
	})
}
