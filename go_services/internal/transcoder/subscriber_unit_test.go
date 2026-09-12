//go:build unit

package transcoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPayload(t *testing.T, jobID string) []byte {
	t.Helper()
	data, err := json.Marshal(VideoChunkMessage{
		JobID:            jobID,
		ChunkIndex:       0,
		StorageURL:       "http://localhost:1/job-1/chunk.mp4",
		TargetResolution: "720p",
	})
	require.NoError(t, err)
	return data
}

func TestConsumeVideoChunkU(t *testing.T) {

	t.Run("successful chunk processing writes the processed kv key", func(t *testing.T) {
		const jobID = "job-abc"
		const chunkIndex = 2
		chunkName := fmt.Sprintf("%s-%d", jobID, chunkIndex)

		t.Cleanup(func() {
			transcodeVideo = TranscodeVideo
			os.RemoveAll("/tmp/temp-unprocessed-" + chunkName)
			os.RemoveAll("/tmp/temp-processed-" + chunkName)
		})

		storageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte("fake source video"))
			}
		}))
		t.Cleanup(storageSrv.Close)

		transcodeVideo = func(_, _, chunkName string, _ *slog.Logger) (string, error) {
			outDir := "/tmp/temp-processed-" + chunkName
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return "", err
			}
			outputPath := filepath.Join(outDir, "chunk.mp4")
			if err := os.WriteFile(outputPath, []byte("fake transcoded output"), 0644); err != nil {
				return "", err
			}
			return outputPath, nil
		}

		payload, err := json.Marshal(VideoChunkMessage{
			JobID:            jobID,
			ChunkIndex:       chunkIndex,
			TotalChunks:      1,
			StorageURL:       storageSrv.URL + "/chunk.mp4",
			TargetResolution: "480p",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		processedKV := &test.MockKV{}

		_, err = ConsumeVideoChunk(
			storageSrv.URL, &test.MockDrainer{}, js, processedKV, &test.MockKV{}, &test.MockKV{},
			30*time.Second, test.SilentLogger(),
		)
		require.NoError(t, err)

		assert.True(t, msg.AckCalled, "expected successful processing to ack the message")
		assert.False(t, msg.NakCalled, "expected successful processing not to nak the message")
		assert.Equal(t, fmt.Sprintf("%s.%d", jobID, chunkIndex), processedKV.PutKey)
	})

	// failure tests
	t.Run("Consume fail returns error", func(t *testing.T) {
		consumeErr := errors.New("consume error")

		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

		_, err := ConsumeVideoChunk("http://storage", nil, js, &test.MockKV{}, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumeErr)
	})

	t.Run("fetch failure should nak", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		_, err := ConsumeVideoChunk("http://storage", nil, js, &test.MockKV{}, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.NakCalled)
	})

	// idempotency test cases

	t.Run("already processed chunk acks and skips processing", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := ConsumeVideoChunk("http://storage", nil, js, kv, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("already processed chunk does not write to kv again", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := ConsumeVideoChunk("http://storage", nil, js, kv, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv check error does not ack or nak", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := ConsumeVideoChunk("http://storage", nil, js, kv, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("does not write kv when chunk fetch fails", func(t *testing.T) {
		payload, err := json.Marshal(VideoChunkMessage{
			JobID:            "job-abc",
			ChunkIndex:       2,
			StorageURL:       "http://localhost:1/job-abc/chunk.mp4",
			TargetResolution: "480p",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{}

		_, _ = ConsumeVideoChunk("http://localhost:1", nil, js, kv, &test.MockKV{}, &test.MockKV{}, 30*time.Second, test.SilentLogger())

		assert.Empty(t, kv.PutKey, "kv.Put should not be called when processing fails")
	})

	// cancelled test cases

	t.Run("cancelled job terminates message and does no other work", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		processedKV := &test.MockKV{}
		jobMilestoneKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"transcoder"}`),
		}
		claimKV := &test.MockKV{}

		_, err := ConsumeVideoChunk("http://storage", nil, js, processedKV, jobMilestoneKV, claimKV, 15*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Empty(t, processedKV.PutKey, "cancelled gate should bail before dedupe/combine work")
		assert.Empty(t, claimKV.CreateKey, "cancelled gate should never claim the chunk")
	})

	t.Run("failing to terminate a msg on cancelled job is logged not naked", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1"), TermErr: errors.New("term failed")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		jobMilestoneKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"transcoder"}`),
		}

		_, err := ConsumeVideoChunk("http://storage", nil, js, &test.MockKV{}, jobMilestoneKV, &test.MockKV{}, 15*time.Second, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.NakCalled)
	})
}
