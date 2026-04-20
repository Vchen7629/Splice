package test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

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

	return endpoint, func() { _ = container.Terminate(ctx) }
}

const testVideoPath = "../test/testvideo.mp4"

// helper to open the test video
func OpenTestVideo(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(testVideoPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := f.Close()
		require.NoError(t, err)
	})
	return f
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
