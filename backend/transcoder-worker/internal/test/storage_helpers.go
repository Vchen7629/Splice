//go:build integration

package test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const testVideoPath = "../test/test_video.mp4"

// helper to open the test video
func OpenTestVideo(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(testVideoPath)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

func SeedUnprocessedVideo(t *testing.T, filerURL, jobID, fileName string, content []byte) string {
	t.Helper()
	url := fmt.Sprintf("%s/%s/%s", filerURL, jobID, fileName)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Less(t, resp.StatusCode, 400)
	return url
}

func SeedProcessedVideo(t *testing.T, filerURL, jobID, fileName string, content []byte) {
	t.Helper()
	url := fmt.Sprintf("%s/%s/processed/%s", filerURL, jobID, fileName)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Less(t, resp.StatusCode, 400)
}
