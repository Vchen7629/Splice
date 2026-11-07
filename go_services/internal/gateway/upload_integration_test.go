//go:build integration

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/test"
)

func newUploadHandler(js jetstream.JetStream, kv jetstream.KeyValue, filerURL string) *videoHandler {
	return &videoHandler{
		logger:     test.SilentLogger(),
		js:         js,
		kv:         kv,
		storageURL: filerURL,
	}
}

func TestUploadVideoRouteI(t *testing.T) {
	env := setupServer(t)

	t.Run("transcode upload writes PROCESSING state to KV and publishes chunk message", func(t *testing.T) {
		received := make(chan []byte, 1)
		sub, err := env.nc.Subscribe("jobs.video.chunks", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		req := NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", testVideoBytes(t), "720p", "transcode")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
		require.NotEmpty(t, uploadResp.JobID)

		kv := test.SetupJobMilestoneKV(t, env.js)
		entry, err := kv.Get(t.Context(), uploadResp.JobID)
		require.NoError(t, err)
		var status struct {
			State string `json:"state"`
			Stage string `json:"stage"`
		}
		require.NoError(t, json.Unmarshal(entry.Value(), &status))
		assert.Equal(t, "PROCESSING", status.State)
		assert.Equal(t, "upload", status.Stage)

		select {
		case data := <-received:
			var msg pipeline.VideoJobMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("multiple uploads each get their own PROCESSING entry in KV", func(t *testing.T) {
		kv := test.SetupJobMilestoneKV(t, env.js)
		jobIDs := make([]string, 3)

		for i := range jobIDs {
			req := NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", testVideoBytes(t), "1080p", "transcode")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close() //nolint:errcheck
			require.Equal(t, http.StatusCreated, resp.StatusCode)

			var uploadResp struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
			jobIDs[i] = uploadResp.JobID
		}

		for _, jobID := range jobIDs {
			entry, err := kv.Get(t.Context(), jobID)
			require.NoError(t, err, "KV entry missing for job %s", jobID)
			var status struct {
				State string `json:"state"`
			}
			require.NoError(t, json.Unmarshal(entry.Value(), &status))
			assert.Equal(t, "PROCESSING", status.State)
		}
	})

	t.Run("Published NATS message contains correct job_id, target_resolution, storage_url, and operation", func(t *testing.T) {
		kv := test.SetupJobMilestoneKV(t, env.js)
		h := newUploadHandler(env.js, kv, sharedFilerUrl)
		patchProbeVideo(t)

		received := make(chan []byte, 1)
		sub, err := env.nc.Subscribe("jobs.video.chunks", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		defer sub.Unsubscribe() //nolint:errcheck

		req := NewUploadRequest(t, "/jobs", "video.mp4", testVideoBytes(t), "720p", "transcode")
		rec := httptest.NewRecorder()
		h.uploadVideoRoute(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&uploadResp))

		select {
		case data := <-received:
			var msg pipeline.VideoChunkMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
			assert.Equal(t, "transcode", msg.Operation)
			assert.Equal(t, fmt.Sprintf("%s/%s/video_chunk_0000.mkv", sharedFilerUrl, uploadResp.JobID), msg.StorageURL)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})
}
