//go:build unit

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	valid := videoInfo{format: "mov,mp4,m4a,3gp,3g2,mj2", width: 1280, height: 720, packets: 100}

	t.Run("accepts a valid video", func(t *testing.T) {
		assert.NoError(t, validateVideo(valid, 1))
	})

	t.Run("accepts if any part of the comma joined format is allowed", func(t *testing.T) {
		info := valid
		info.format = "avi"

		assert.NoError(t, validateVideo(info, 1))
	})

	tt := []struct {
		name       string
		modify     func(*videoInfo)
		size       int64
		wantReason string
	}{
		{"empty file", func(*videoInfo) {}, 0, "empty file"},
		{"unsupported container", func(i *videoInfo) { i.format = "flv" }, 1, "unsupported video format"},
		{"empty format", func(i *videoInfo) { i.format = "" }, 1, "unsupported video format"},
		{"zero width", func(i *videoInfo) { i.width = 0 }, 1, "video is empty"},
		{"zero height", func(i *videoInfo) { i.height = 0 }, 1, "video is empty"},
		{"zero packets", func(i *videoInfo) { i.packets = 0 }, 1, "video is empty"},
		{"resolution too large", func(i *videoInfo) { i.width, i.height = 8200, 4700 }, 1, "video resolution too large"},
		{"too many total pixels", func(i *videoInfo) { i.packets = 1e9 }, 1, "video too large to process"},
	}
	for _, tc := range tt {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			info := valid
			tc.modify(&info)

			assertRejected(t, validateVideo(info, tc.size), tc.wantReason)
		})
	}

	t.Run("accepts the largest allowed frame", func(t *testing.T) {
		info := valid
		info.width, info.height, info.packets = 8192, 4608, 1

		assert.NoError(t, validateVideo(info, 1))
	})
}

func TestValidateUploadFields(t *testing.T) {
	tt := []struct {
		name, targetRes, processType, denoiseQuality, filename string
	}{
		{"missing target_resolution field", "", "transcode", "", "somefile"},
		{"missing process_type field", "1080p", "", "", "somefile"},
		{"invalid process_type field", "1080p", "IDK", "", "somefile"},
		{"invalid video filename", "1080p", "transcode", "", "../etc/passwd"}, // path traversal
		{"invalid video filename", "1080p", "transcode", "", ""},
		{"uploaded file target res not allowed", "9999p", "transcode", "", "somefile"},
		{"specified denoise quality not allowed", "1080p", "denoise", "heavyxl", "somefile"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUploadFields(tc.filename, tc.targetRes, tc.processType, tc.denoiseQuality)

			assert.Error(t, err)
			assert.Equal(t, tc.name, err.Error())
		})
	}

	t.Run("denoise and convert ignore resolution fields", func(t *testing.T) {
		assert.NoError(t, validateUploadFields("somefilename", "", "denoise", "light"))
		assert.NoError(t, validateUploadFields("somefilename", "", "convert", ""))
		assert.NoError(t, validateUploadFields("somefilename", "1080p", "denoise", "light")) // ignored, not validated
	})

	t.Run("valid fields returns no error", func(t *testing.T) {
		assert.NoError(t, validateUploadFields("somefilename", "1080p", "transcode", ""))
	})
}
