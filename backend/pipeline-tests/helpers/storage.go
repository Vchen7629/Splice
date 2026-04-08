//go:build integration

package helpers

import (
	"context"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

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
