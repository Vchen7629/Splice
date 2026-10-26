//go:build integration

package converter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

// starts the convert consumer on a fresh nats instance
func startConsumer(t *testing.T, baseStorageURL string) (jetstream.JetStream, *nats.Conn) {
	t.Helper()
	js, nc := test.SetupNats(t)
	cc, err := ConsumeAndProcessVideo(
		baseStorageURL, js, test.SetupKV(t, js, "converter-job-processed"), test.SetupJobMilestoneKV(t, js), test.SilentLogger(),
	)
	require.NoError(t, err)
	t.Cleanup(cc.Stop)
	return js, nc
}

// seeds content as the job's source video and publishes a convert job for it
func publishConvertJob(t *testing.T, js jetstream.JetStream, jobID, targetFormat string, content []byte) {
	t.Helper()
	t.Cleanup(func() {
		os.RemoveAll(storage.TempUnprocessedDir(jobID))
		os.RemoveAll("/tmp/convert/" + jobID)
	})
	storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "clip.mp4", content)

	data, err := json.Marshal(pipeline.VideoJobMessage{JobID: jobID, StorageURL: storageURL, TargetFormat: targetFormat})
	require.NoError(t, err)
	_, err = js.Publish(context.Background(), "jobs.video.convert", data)
	require.NoError(t, err)
}

func TestConsumeAndProcessVideoI(t *testing.T) {
	ctx := context.Background()
	clip, err := os.ReadFile(makeTestClip(t, true))
	require.NoError(t, err)

	t.Run("consumer is created with correct config", func(t *testing.T) {
		js, _ := startConsumer(t, sharedFilerURL)

		cons, err := js.Consumer(ctx, "jobs", "convert-worker")
		require.NoError(t, err)
		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "jobs.video.convert", info.Config.FilterSubject)
		assert.Equal(t, time.Hour, info.Config.AckWait)
		assert.Equal(t, 1, info.Config.MaxAckPending)
	})

	t.Run("converted video is uploaded to the download path and job completes", func(t *testing.T) {
		js, nc := startConsumer(t, sharedFilerURL)
		completes := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.complete", func(m *nats.Msg) { completes <- m.Data })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		publishConvertJob(t, js, "job-full-flow", "webm", clip)

		select {
		case data := <-completes:
			var msg pipeline.JobCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, "job-full-flow", msg.JobID)
		case <-time.After(60 * time.Second):
			t.Fatal("timed out waiting for jobs.complete")
		}

		// same path the gateway downloads from
		resp, err := http.Get(sharedFilerURL + "/job-full-flow/output.webm/processed")
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	nakTests := []struct {
		name           string
		baseStorageURL string
		content        []byte
	}{
		{"convert failure naks for redelivery", sharedFilerURL, []byte("not a video")},
		{"upload failure naks for redelivery", "http://localhost:1", clip},
	}

	for i, tc := range nakTests {
		t.Run(tc.name, func(t *testing.T) {
			js, _ := startConsumer(t, tc.baseStorageURL)

			publishConvertJob(t, js, fmt.Sprintf("job-nak-%d", i), "mp4", tc.content)

			require.Eventually(t, func() bool {
				cons, err := js.Consumer(ctx, "jobs", "convert-worker")
				if err != nil {
					return false
				}
				info, err := cons.Info(ctx)
				return err == nil && info.NumRedelivered > 0
			}, 30*time.Second, 200*time.Millisecond, "expected msg to be nacked and redelivered")
		})
	}
}
