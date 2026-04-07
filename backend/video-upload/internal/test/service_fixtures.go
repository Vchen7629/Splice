package test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// NewUploadRequest builds a multipart POST request.
// Pass a path ("/jobs") for direct handler invocation via httptest.NewRecorder,
// or a full URL ("http://host:port/jobs") for use with http.DefaultClient.
// Pass filename="" to omit the video field. Pass targetRes="" to omit target_resolution.
func NewUploadRequest(t *testing.T, target, filename string, fileContent []byte, targetRes string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if filename != "" {
		fw, err := w.CreateFormFile("video", filename)
		require.NoError(t, err)
		_, err = fw.Write(fileContent)
		require.NoError(t, err)
	}

	if targetRes != "" {
		require.NoError(t, w.WriteField("target_resolution", targetRes))
	}

	require.NoError(t, w.Close())

	req, err := http.NewRequest(http.MethodPost, target, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// for main.go unit tests
func WriteEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "STORAGE_URL", "HTTP_PORT"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() {
				err := os.Setenv(key, old)
				require.NoError(t, err)
			})
		} else {
			t.Cleanup(func() {
				err := os.Unsetenv(key)
				require.NoError(t, err)
			})
		}
		err := os.Unsetenv(key)
		require.NoError(t, err)
	}
	path := filepath.Join("..", ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove(path) })
}
