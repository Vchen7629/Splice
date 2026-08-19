//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
	"splice.com/go_services/internal/shared/test"
)

// Shared containers for every integration test in this package. TestMain below
// is the only one in the test binary, so package gateway_test files (cors) run
// under it too.
var (
	sharedJS       jetstream.JetStream
	sharedNC       *nats.Conn
	sharedKV       jetstream.KeyValue
	sharedFilerUrl string
)

// JobStatus mirrors handler.JobStatus for KV assertion helpers.
// Kept minimal — only the fields needed for assertions.
type AssertJobStatus struct {
	State string `json:"state"`
	Stage string `json:"stage"`
	Error string `json:"error,omitempty"`
}

// helpers

// assertKVFailed polls the KV until the entry for jobID has state FAILED, then checks the error field contains wantErrContains.
func assertKVFailed(t *testing.T, kv jetstream.KeyValue, jobID, wantErrContains string) {
	t.Helper()
	require.Eventually(t, func() bool {
		entry, err := kv.Get(context.Background(), jobID)
		if err != nil {
			return false
		}
		var s AssertJobStatus
		return json.Unmarshal(entry.Value(), &s) == nil && s.State == "FAILED"
	}, 5*time.Second, 100*time.Millisecond, "KV entry for %q never reached FAILED state", jobID)

	entry, err := kv.Get(context.Background(), jobID)
	require.NoError(t, err)
	var s AssertJobStatus
	require.NoError(t, json.Unmarshal(entry.Value(), &s))
	assert.Contains(t, s.Error, wantErrContains)
}

// assertKVEmpty waits briefly then asserts the key is absent from the KV.
func assertKVEmpty(t *testing.T, kv jetstream.KeyValue, jobID string) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	_, err := kv.Get(context.Background(), jobID)
	assert.True(t, errors.Is(err, jetstream.ErrKeyNotFound), "expected KV entry for %q to be absent, got err: %v", jobID, err)
}

// AssertKVComplete polls the KV until the entry for jobID has state COMPLETE.
func assertKVComplete(t *testing.T, kv jetstream.KeyValue, jobID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		entry, err := kv.Get(context.Background(), jobID)
		if err != nil {
			return false
		}
		var s AssertJobStatus
		return json.Unmarshal(entry.Value(), &s) == nil && s.State == "COMPLETE"
	}, 5*time.Second, 100*time.Millisecond, "KV entry for %q never reached COMPLETE state", jobID)
}

// Publishes payload to subject and returns the assigned stream sequence number.
func seedStreamMessage(t *testing.T, js jetstream.JetStream, subject string, payload []byte) uint64 {
	t.Helper()
	ack, err := js.Publish(context.Background(), subject, payload)
	require.NoError(t, err)
	return ack.Sequence
}

// Mirrors the max-delivery advisory shape that JetStream publishes.
type advisoryPayload struct {
	Stream    string `json:"stream"`
	Consumer  string `json:"consumer"`
	StreamSeq uint64 `json:"stream_seq"`
}

// Publishes a fake max-delivery advisory to core NATS so the handler callback fires immediately.
func publishAdvisory(t *testing.T, nc *nats.Conn, stream, consumer string, seq uint64) {
	t.Helper()
	subject := "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES." + stream
	data, err := json.Marshal(advisoryPayload{Stream: stream, Consumer: consumer, StreamSeq: seq})
	require.NoError(t, err)
	require.NoError(t, nc.Publish(subject, data))
}

// startNats starts a NATS container with JetStream enabled and creates the jobs stream.
func startNats() (jetstream.JetStream, *nats.Conn, func()) {
	ctx := context.Background()

	container, err := natstc.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		panic("failed to start NATS container: " + err.Error())
	}

	url, err := container.ConnectionString(ctx)
	if err != nil {
		panic("failed to get NATS connection string: " + err.Error())
	}

	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(200*time.Millisecond),
	)
	if err != nil {
		panic("failed to connect to NATS: " + err.Error())
	}

	js, err := jetstream.New(nc)
	if err != nil {
		panic("failed to create JetStream context: " + err.Error())
	}

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "jobs",
		Subjects: []string{"jobs.>"},
	})
	if err != nil {
		panic("failed to create jobs stream: " + err.Error())
	}

	return js, nc, func() {
		nc.Close()
		_ = container.Terminate(ctx)
	}
}

// createKV creates the job-status KV bucket. Only TestMain needs it.
func createKV(js jetstream.JetStream) jetstream.KeyValue {
	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "job-status",
	})
	if err != nil {
		panic("failed to create job-status KV bucket: " + err.Error())
	}
	return kv
}

func TestMain(m *testing.M) {
	var natsCleanup func()
	sharedJS, sharedNC, natsCleanup = startNats()
	sharedKV = createKV(sharedJS)

	filerURL, filerCleanup := test.StartSeaweedFSFiler()
	sharedFilerUrl = filerURL

	code := m.Run()

	filerCleanup()
	natsCleanup()
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
			seq := seedStreamMessage(t, sharedJS, tc.subject, mustMarshalJob(t, jobID))
			publishAdvisory(t, sharedNC, "jobs", tc.consumer, seq)

			assertKVFailed(t, sharedKV, jobID, tc.wantErrContains)
		})
	}
}

// covers cases where the advisory handler encounters an error mid-way and leaves the KV unwritten.
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
				seedStreamMessage(t, sharedJS, "jobs.video.chunks", mustMarshalJob(t, "irrelevant"))
				return "jobs", "transcoder-worker", 99999
			},
		},
		{
			name:  "original message payload is not valid JSON",
			jobID: "job-bad-payload",
			seed: func(t *testing.T) (string, string, uint64) {
				seq := seedStreamMessage(t, sharedJS, "jobs.video.chunks", []byte("not valid json{{"))
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
				publishAdvisory(t, sharedNC, stream, consumer, seq)
			}

			assertKVEmpty(t, sharedKV, tc.jobID)
		})
	}
}

func TestListenAdvisoriesFailure_KVPutFails(t *testing.T) {
	t.Run("KV Put failure is handled without panic", func(t *testing.T) {
		mockKV := NewMockKV()
		mockKV.PutErr = errors.New("kv unavailable")

		sub, err := ListenAdvisoriesFailure(sharedNC, sharedJS, mockKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		jobID := "job-kv-fail"
		seq := seedStreamMessage(t, sharedJS, "jobs.video.chunks", mustMarshalJob(t, jobID))
		publishAdvisory(t, sharedNC, "jobs", "transcoder-worker", seq)

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

		nc, err := nats.Connect(url,
			nats.MaxReconnects(5),
			nats.RetryOnFailedConnect(true),
			nats.ReconnectWait(200*time.Millisecond),
		)
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

		assertKVComplete(t, sharedKV, jobID)
	})

	t.Run("invalid JSON does not write KV", func(t *testing.T) {
		consCtx, err := ListenJobComplete(sharedJS, sharedKV, test.SilentLogger())
		require.NoError(t, err)
		t.Cleanup(consCtx.Stop)

		_, err = sharedJS.Publish(context.Background(), "jobs.complete", []byte("not valid json{{"))
		require.NoError(t, err)

		assertKVEmpty(t, sharedKV, "jc-bad-json")
	})

	t.Run("KV Put failure is handled without panic", func(t *testing.T) {
		mockKV := NewMockKV()
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
