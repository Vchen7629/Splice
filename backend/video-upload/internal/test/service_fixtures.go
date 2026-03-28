package test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// builds a multipart POST request. Pass filename="" to omit the video field entirely.
// Pass targetRes="" to omit the target_resolution field.
func NewUploadRequest(t *testing.T, filename string, fileContent []byte, targetRes string) *http.Request {
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

	req := httptest.NewRequest(http.MethodPost, "/videos", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}
