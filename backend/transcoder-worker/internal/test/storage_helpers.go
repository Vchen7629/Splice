//go:build integration

package test

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

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
