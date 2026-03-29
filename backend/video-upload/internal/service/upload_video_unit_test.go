//go:build unit

package service_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"video-upload/internal/service"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func TestErrorCases(t *testing.T) {
	t.Run("Returns wrapped error when job directory cannot be created", func(t *testing.T) {
		result, err := service.SaveUploadedVideo(
			strings.NewReader("data"), "\x00", "video.mp4", test.SilentLogger(),
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create job dir err")
		assert.Empty(t, result.JobID)
		assert.Empty(t, result.StoragePath)
	})

	t.Run("Returns wrapped error when destination file cannot be created", func(t *testing.T) {
		dir := t.TempDir()

		// null byte in filename causes os.Create to fail after MkdirAll succeeds
		result, err := service.SaveUploadedVideo(
			strings.NewReader("data"), dir, "video\x00.mp4", test.SilentLogger(),
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "create video file error")
		assert.Empty(t, result.JobID)
		assert.Empty(t, result.StoragePath)
	})

	t.Run("Returns wrapped error when reading from src fails", func(t *testing.T) {
		dir := t.TempDir()

		result, err := service.SaveUploadedVideo(&errorReader{}, dir, "video.mp4", test.SilentLogger())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write video error")
		assert.Empty(t, result.JobID)
		assert.Empty(t, result.StoragePath)
	})
}

func TestSuccessResult(t *testing.T) {
	t.Run("Returns non-empty JobID and StoragePath on success", func(t *testing.T) {
		dir := t.TempDir()

		result, err := service.SaveUploadedVideo(strings.NewReader("data"), dir, "clip.mp4", test.SilentLogger())

		require.NoError(t, err)
		assert.NotEmpty(t, result.JobID)
		assert.NotEmpty(t, result.StoragePath)
		assert.Contains(t, result.StoragePath, result.JobID)
		assert.True(t, strings.HasSuffix(result.StoragePath, "clip.mp4"))
	})

	t.Run("Strips path traversal from filename via filepath.Base", func(t *testing.T) {
		dir := t.TempDir()

		result, err := service.SaveUploadedVideo(strings.NewReader("data"), dir, "../../../etc/passwd", test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(result.StoragePath, "passwd"))
		assert.NotContains(t, result.StoragePath, "..")
	})
}

func TestFileSystem(t *testing.T) {
	t.Run("File is saved at StoragePath with correct content", func(t *testing.T) {
		dir := t.TempDir()
		content := []byte("fake video bytes")

		result, err := service.SaveUploadedVideo(bytes.NewReader(content), dir, "test.mp4", test.SilentLogger())
		require.NoError(t, err)

		saved, err := os.ReadFile(result.StoragePath)
		require.NoError(t, err)
		assert.Equal(t, content, saved)
	})

	t.Run("StoragePath follows storageDir/jobs/jobID/filename structure", func(t *testing.T) {
		dir := t.TempDir()

		result, err := service.SaveUploadedVideo(strings.NewReader("data"), dir, "video.mp4", test.SilentLogger())
		require.NoError(t, err)

		expected := filepath.Join(dir, "jobs", result.JobID, "video.mp4")
		assert.Equal(t, expected, result.StoragePath)
	})

	t.Run("Each call produces a unique JobID and path", func(t *testing.T) {
		dir := t.TempDir()

		result1, err1 := service.SaveUploadedVideo(strings.NewReader("data1"), dir, "video.mp4", test.SilentLogger())
		result2, err2 := service.SaveUploadedVideo(strings.NewReader("data2"), dir, "video.mp4", test.SilentLogger())

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, result1.JobID, result2.JobID)
		assert.NotEqual(t, result1.StoragePath, result2.StoragePath)
	})

	t.Run("File is closed after write and can be reopened", func(t *testing.T) {
		dir := t.TempDir()

		result, err := service.SaveUploadedVideo(strings.NewReader("data"), dir, "video.mp4", test.SilentLogger())
		require.NoError(t, err)

		f, err := os.Open(result.StoragePath)
		require.NoError(t, err)
		f.Close()
	})
}
