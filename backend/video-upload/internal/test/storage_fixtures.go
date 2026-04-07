//go:build integration

package test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Starts a SeaweedFS container and returns its master URL.
func SetupSeaweedFS(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "chrislusf/seaweedfs",
		Cmd:          []string{"server", "-dir=/data", "-master.port=9333", "-volume.port=8080"},
		ExposedPorts: []string{"9333/tcp"},
		WaitingFor:   wait.ForHTTP("/dir/status").WithPort("9333/tcp").WithStatusCodeMatcher(func(status int) bool { return status < 500 }),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	endpoint, err := container.PortEndpoint(ctx, "9333/tcp", "http")
	require.NoError(t, err)

	return endpoint
}

// SetupSeaweedFSFiler starts a SeaweedFS container with the filer enabled and
// returns the filer URL (port 8888). Waits for both master and filer to be
// ready before returning so the volume is available for writes.
func SetupSeaweedFSFiler(t *testing.T) string {
	t.Helper()
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
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	endpoint, err := container.PortEndpoint(ctx, "8888/tcp", "http")
	require.NoError(t, err)

	return endpoint
}

const testVideoPath = "../test/testvideo.mp4"

// helper to open the test video
func OpenTestVideo(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(testVideoPath)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}
