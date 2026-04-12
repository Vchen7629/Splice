//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"
	"video-status/internal/handler"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestMainI(t *testing.T) {
	t.Run("graceful shutdown on SIGTERM", func(t *testing.T) {
		js, nc, cleanup := test.StartNats()
		defer cleanup()

		httpPort := freePort(t)
		t.Setenv("NATS_URL", nc.ConnectedUrl())
		t.Setenv("HTTP_PORT", fmt.Sprintf("%d", httpPort))

		kv := test.CreateKV(js)
		b, err := json.Marshal(handler.JobStatus{State: handler.StateProcessing})
		require.NoError(t, err)
		_, err = kv.Put(context.Background(), "test-job", b)
		require.NoError(t, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			main()
		}()

		baseURL := fmt.Sprintf("http://localhost:%d", httpPort)
		require.Eventually(t, func() bool {
			conn, err := net.Dial("tcp", fmt.Sprintf(":%d", httpPort))
			if err == nil {
				conn.Close()
				return true
			}
			return false
		}, 5*time.Second, 20*time.Millisecond, "server did not start in time")

		resp, err := http.Get(baseURL + "/jobs/test-job/status")
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("main() did not shut down after SIGTERM")
		}
	})

	t.Run("exits with code 1 when jobs stream is missing", func(t *testing.T) {
		ctx := context.Background()
		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		url, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		t.Setenv("NATS_URL", url)
		exitCode := patchOsExit(t)
		done := make(chan struct{})

		go func() {
			defer close(done)
			main()
		}()

		select {
		case <-done:
			assert.Equal(t, 1, *exitCode)
		case <-time.After(15 * time.Second):
			t.Fatal("main() did not call osExit in time")
		}
	})
}

func TestStartHttpApiI(t *testing.T) {
	t.Run("shutdown", func(t *testing.T) {
		tests := []struct {
			name    string
			timeout time.Duration
		}{
			{"generous timeout", 5 * time.Second},
			{"tight timeout", 100 * time.Millisecond},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				baseURL, server := startTestServer(t, test.NewMockKV())

				resp, err := http.Get(baseURL + "/jobs/any/status")
				require.NoError(t, err)
				resp.Body.Close()

				ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
				defer cancel()

				assert.NoError(t, server.Shutdown(ctx))

				_, err = http.Get(baseURL + "/jobs/any/status")
				assert.Error(t, err, "expected connection refused after shutdown")
			})
		}
	})

	t.Run("serves route end-to-end", func(t *testing.T) {
		tests := []struct {
			name       string
			jobID      string
			seed       *handler.JobStatus
			wantStatus int
			wantState  string
		}{
			{
				name:       "seeded job returns 200 with correct state",
				jobID:      "job-processing",
				seed:       &handler.JobStatus{State: handler.StateProcessing},
				wantStatus: http.StatusOK,
				wantState:  "PROCESSING",
			},
			{
				name:       "unknown job returns 404",
				jobID:      "job-missing",
				wantStatus: http.StatusNotFound,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				kv := test.NewMockKV()
				if tc.seed != nil {
					b, err := json.Marshal(tc.seed)
					require.NoError(t, err)
					kv.Seed(tc.jobID, b)
				}

				baseURL, server := startTestServer(t, kv)
				t.Cleanup(func() { server.Close() })

				resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", baseURL, tc.jobID))
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, tc.wantStatus, resp.StatusCode)

				if tc.wantState != "" {
					var body struct {
						State string `json:"state"`
					}
					require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
					assert.Equal(t, tc.wantState, body.State)
				}
			})
		}
	})
}
