//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
	"video-status/internal/test"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

var (
	sharedJS jetstream.JetStream
	sharedNC *nats.Conn
	sharedKV jetstream.KeyValue
)

func TestMain(m *testing.M) {
	var cleanup func()
	sharedJS, sharedNC, cleanup = test.StartNats()
	sharedKV = test.CreateKV(sharedJS)

	code := m.Run()

	cleanup()
	os.Exit(code)
}

type jobMsg struct {
	JobID string `json:"job_id"`
}

func mustMarshalJob(t *testing.T, jobID string) []byte {
	t.Helper()
	b, err := json.Marshal(jobMsg{JobID: jobID})
	require.NoError(t, err)
	return b
}

// panic variant for use in table literal field initializers.
func mustMarshalJobStatic(jobID string) []byte {
	b, err := json.Marshal(jobMsg{JobID: jobID})
	if err != nil {
		panic(err)
	}
	return b
}

func TestListenAdvisoriesFailure_ReturnsSub(t *testing.T) {
	sub, err := ListenAdvisoriesFailure(sharedNC, sharedJS, sharedKV, test.SilentLogger())

	require.NoError(t, err)
	assert.NotNil(t, sub)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
}

func TestListenAdvisoriesFailure_WritesKV(t *testing.T) {
	tests := []struct {
		name            string
		subject         string
		consumer        string
		wantErrContains string
	}{
		{
			name:            "writes FAILED for transcoder-worker advisory",
			subject:         "jobs.video.chunks",
			consumer:        "transcoder-worker",
			wantErrContains: "transcoder-worker",
		},
		{
			name:            "writes FAILED for video-recombiner advisory",
			subject:         "jobs.complete",
			consumer:        "video-recombiner",
			wantErrContains: "video-recombiner",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := ListenAdvisoriesFailure(sharedNC, sharedJS, sharedKV, test.SilentLogger())
			require.NoError(t, err)
			t.Cleanup(func() { _ = sub.Unsubscribe() })

			jobID := "job-" + tc.consumer
			seq := test.SeedStreamMessage(t, sharedJS, tc.subject, mustMarshalJob(t, jobID))
			test.PublishAdvisory(t, sharedNC, "jobs", tc.consumer, seq)

			test.AssertKVFailed(t, sharedKV, jobID, tc.wantErrContains)
		})
	}
}

// TestListenAdvisoriesFailure_Ignored covers cases where the advisory handler
// encounters an error mid-way and leaves the KV unwritten.
func TestListenAdvisoriesFailure_Ignored(t *testing.T) {
	tests := []struct {
		name  string
		jobID string
		seed  func(t *testing.T) (stream, consumer string, seq uint64)
	}{
		{
			name:  "invalid advisory JSON",
			jobID: "job-bad-advisory",
			seed: func(t *testing.T) (string, string, uint64) {
				require.NoError(t, sharedNC.Publish("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.jobs", []byte("not json{{")))
				return "", "", 0
			},
		},
		{
			name:  "stream referenced in advisory does not exist",
			jobID: "job-no-stream",
			seed: func(_ *testing.T) (string, string, uint64) {
				return "nonexistent-stream", "some-consumer", 1
			},
		},
		{
			name:  "sequence referenced in advisory does not exist",
			jobID: "job-bad-seq",
			seed: func(t *testing.T) (string, string, uint64) {
				test.SeedStreamMessage(t, sharedJS, "jobs.video.chunks", mustMarshalJob(t, "irrelevant"))
				return "jobs", "transcoder-worker", 99999
			},
		},
		{
			name:  "original message payload is not valid JSON",
			jobID: "job-bad-payload",
			seed: func(t *testing.T) (string, string, uint64) {
				seq := test.SeedStreamMessage(t, sharedJS, "jobs.video.chunks", []byte("not valid json{{"))
				return "jobs", "transcoder-worker", seq
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := ListenAdvisoriesFailure(sharedNC, sharedJS, sharedKV, test.SilentLogger())
			require.NoError(t, err)
			t.Cleanup(func() { _ = sub.Unsubscribe() })

			stream, consumer, seq := tc.seed(t)
			if stream != "" {
				test.PublishAdvisory(t, sharedNC, stream, consumer, seq)
			}

			test.AssertKVEmpty(t, sharedKV, tc.jobID)
		})
	}
}

func TestListenAdvisoriesFailure_KVPutFails(t *testing.T) {
	t.Run("KV Put failure is handled without panic", func(t *testing.T) {
		mockKV := test.NewMockKV()
		mockKV.PutErr = errors.New("kv unavailable")

		sub, err := ListenAdvisoriesFailure(sharedNC, sharedJS, mockKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		jobID := "job-kv-fail"
		seq := test.SeedStreamMessage(t, sharedJS, "jobs.video.chunks", mustMarshalJob(t, jobID))
		test.PublishAdvisory(t, sharedNC, "jobs", "transcoder-worker", seq)

		require.Eventually(t, func() bool {
			return mockKV.PutCalled
		}, 5*time.Second, 100*time.Millisecond, "expected KV Put to be called")
	})
}

func TestListenJobCompleteI(t *testing.T) {
	t.Run("returns error when no stream covers jobs.complete", func(t *testing.T) {
		ctx := context.Background()

		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		url, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		nc, err := nats.Connect(url)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		_, err = ListenJobComplete(js, sharedKV, test.SilentLogger())

		assert.Error(t, err)
	})
	t.Run("returns sub", func(t *testing.T) {
		consCtx, err := ListenJobComplete(sharedJS, sharedKV, test.SilentLogger())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		t.Cleanup(consCtx.Stop)
	})
	t.Run("Consumer config", func(t *testing.T) {
		ctx := context.Background()

		consCtx, err := ListenJobComplete(sharedJS, sharedKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(consCtx.Stop)

		stream, err := sharedJS.Stream(ctx, "jobs")
		require.NoError(t, err)
		cons, err := stream.Consumer(ctx, "video-status-complete")
		require.NoError(t, err)
		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "video-status-complete", info.Config.Name)
		assert.Equal(t, "video-status-complete", info.Config.Durable)
		assert.Equal(t, "jobs.complete", info.Config.FilterSubject)
		assert.Equal(t, jetstream.AckExplicitPolicy, info.Config.AckPolicy)
		assert.Equal(t, 3, info.Config.MaxDeliver)
		assert.Equal(t, 30*time.Second, info.Config.AckWait)
	})
}

func TestListenJobComplete(t *testing.T) {
	t.Run("valid jobs.complete message writes COMPLETE to KV and acks", func(t *testing.T) {
		consCtx, err := ListenJobComplete(sharedJS, sharedKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(consCtx.Stop)

		jobID := "job-complete-kv"
		_, err = sharedJS.Publish(context.Background(), "jobs.complete", mustMarshalJob(t, jobID))
		require.NoError(t, err)

		test.AssertKVComplete(t, sharedKV, jobID)
	})

	t.Run("invalid JSON does not write KV", func(t *testing.T) {
		consCtx, err := ListenJobComplete(sharedJS, sharedKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(consCtx.Stop)

		_, err = sharedJS.Publish(context.Background(), "jobs.complete", []byte("not valid json{{"))
		require.NoError(t, err)

		test.AssertKVEmpty(t, sharedKV, "jc-bad-json")
	})

	t.Run("KV Put failure is handled without panic", func(t *testing.T) {
		mockKV := test.NewMockKV()
		mockKV.PutErr = errors.New("kv unavailable")

		consCtx, err := ListenJobComplete(sharedJS, mockKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(consCtx.Stop)

		_, err = sharedJS.Publish(context.Background(), "jobs.complete", mustMarshalJobStatic("jc-kv-fail"))
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return mockKV.PutCalled
		}, 5*time.Second, 100*time.Millisecond, "expected KV Put to be called")
	})
}
