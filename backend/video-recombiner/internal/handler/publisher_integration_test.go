//go:build integration

package handler_test

import (
	"encoding/json"
	"testing"
	"time"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishesCorrectPayload(t *testing.T) {
	js, nc := test.SetupNats(t)

	received := make(chan []byte, 1)
	sub, err := nc.Subscribe("jobs.complete", func(msg *nats.Msg) {
		received <- msg.Data
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	msg := service.VideoProcessingCompleteMessage{
		JobID: "job-1",
	}

	err = handler.PublishVideoProcessingComplete(js, msg)
	require.NoError(t, err)

	select {
	case data := <-received:
		var got service.VideoProcessingCompleteMessage
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, msg, got)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}
