package test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartSeaweedFSFiler starts a SeaweedFS filer container and returns the filer
// URL and a cleanup function. Use this when t.Cleanup is not available (e.g. TestMain).
func StartSeaweedFSFiler() (string, func()) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "chrislusf/seaweedfs",
		Cmd:          []string{"server", "-dir=/data", "-master.port=9333", "-volume.port=8080", "-filer"},
		ExposedPorts: []string{"9333/tcp", "8888/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/dir/status").WithPort("9333/tcp").WithStatusCodeMatcher(func(status int) bool { return status < 500 }),
			wait.ForHTTP("/").WithPort("8888/tcp").WithStatusCodeMatcher(func(status int) bool { return status < 500 }),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic("failed to start SeaweedFS container: " + err.Error())
	}

	endpoint, err := container.PortEndpoint(ctx, "8888/tcp", "http")
	if err != nil {
		panic("failed to get SeaweedFS filer endpoint: " + err.Error())
	}

	cleanup := func() { _ = container.Terminate(ctx) }

	err = waitForFilerWritable(endpoint)
	if err != nil {
		cleanup()
		panic("SeaweedFS filer never became writable: " + err.Error())
	}

	return endpoint, cleanup
}

// polls the filer with real writes until one succeeds. Makes sure seaweedfs is actually
// working to prevent flaky tests on slow infrs
func waitForFilerWritable(filerURL string) error {
	url := filerURL + "/health-check-tmp"
	deadline := time.Now().Add(30*time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte("ok")))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			del, err := http.NewRequest(http.MethodDelete, url, nil)
			if err == nil {
				if delResp, err := http.DefaultClient.Do(del); err == nil {
					_ = delResp.Body.Close()
				}
			}
			return nil
		}

		lastErr = fmt.Errorf("write returned status %d", resp.StatusCode)

		time.Sleep(500 * time.Millisecond)
	}

	return lastErr
}

// OpenTestVideo opens the shared testvideo.mp4 fixture. path is relative to
// the package under test (e.g. "../test/testvideo.mp4" from internal/shared/storage,
// or "../shared/test/testvideo.mp4" from a sibling package like internal/recombiner).
func OpenTestVideo(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := f.Close()
		require.NoError(t, err)
	})
	return f
}

// SeedUnprocessedVideo uploads content to the filer's unprocessed job path
// and returns the storage URL it was written to.
func SeedUnprocessedVideo(t *testing.T, filerURL, jobID, fileName string, content []byte) string {
	t.Helper()
	url := fmt.Sprintf("%s/%s/%s", filerURL, jobID, fileName)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	err = resp.Body.Close()
	require.NoError(t, err)
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

	err = resp.Body.Close()
	require.NoError(t, err)

	require.Less(t, resp.StatusCode, 400)
}
