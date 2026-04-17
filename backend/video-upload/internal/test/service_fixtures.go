package test

import (
	"bytes"
	"mime/multipart"
	"net/http"
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
