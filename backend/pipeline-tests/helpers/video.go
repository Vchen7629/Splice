//go:build integration

package helpers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// generateTestVideo creates a 6s MP4 with a hard red→blue cut so scene-detector produces multiple chunks.
func GenerateTestVideo(t *testing.T, destPath string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=red:duration=3:size=320x240:rate=24",
		"-f", "lavfi", "-i", "color=blue:duration=3:size=320x240:rate=24",
		"-filter_complex", "[0][1]concat=n=2:v=1:a=0",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		destPath,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg failed:\n%s", out)
}

// generateSingleSceneVideo creates a solid-colour video with no scene boundary (1 chunk).
func GenerateSingleSceneVideo(t *testing.T, destPath string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=green:duration=4:size=320x240:rate=24",
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		destPath,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg failed:\n%s", out)
}

func UploadVideo(t *testing.T, baseURL, videoPath, targetResolution string) string {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fw, err := w.CreateFormFile("video", filepath.Base(videoPath))
	require.NoError(t, err)
	data, err := os.ReadFile(videoPath)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.WriteField("target_resolution", targetResolution))
	require.NoError(t, w.Close())

	resp, err := http.Post(baseURL+"/jobs/upload", w.FormDataContentType(), &body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.JobID)
	return result.JobID
}

func DownloadVideo(t *testing.T, baseURL, jobID, fileName string) (*http.Response, error) {
	t.Helper()
	payload, err := json.Marshal(struct {
		JobID    string `json:"job_id"`
		FileName string `json:"file_name"`
	}{JobID: jobID, FileName: fileName})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/jobs/download", strings.NewReader(string(payload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}