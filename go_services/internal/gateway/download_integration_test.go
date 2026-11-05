//go:build integration

package gateway

import (
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVideoPath = "testdata/testvideo.mp4"

// openTestVideo opens the embedded-on-disk test video. Only this file needs it,
// so it is inline rather than in test_helpers_test.go. Note the path is relative
// to the package directory, which is why shared/test's copy ("../test/...") is
// unusable from here.
func openTestVideo(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(testVideoPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := f.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close %s: %v", testVideoPath, err)
		}
	})
	return f
}

func TestGetProcessedVideoI(t *testing.T) {
	t.Run("fetches correct video content from storage", func(t *testing.T) {
		const fileName = "testvideo.mp4"

		expectedBytes, err := os.ReadFile(testVideoPath)
		require.NoError(t, err)

		seedReq, err := http.NewRequest(http.MethodPut, sharedFilerUrl+"/job123/"+fileName+"/processed", openTestVideo(t))
		require.NoError(t, err)

		seedReq.Header.Set("Content-Type", "application/octet-stream")
		seedResp, err := http.DefaultClient.Do(seedReq)
		require.NoError(t, err)

		seedResp.Body.Close()
		require.Less(t, seedResp.StatusCode, 400)

		body, err := getProcessedVideo(sharedFilerUrl, "job123", fileName)
		require.NoError(t, err)
		defer body.Close()

		fetchedBytes, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, expectedBytes, fetchedBytes)
	})
}
