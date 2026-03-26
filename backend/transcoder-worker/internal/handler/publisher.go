package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"transcoder-worker/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const pubSubject = "jobs.chunks.complete"

// Returns a function that publishes a ChunkCompleteMessage to JetStream
// injected into TranscoderService.OnComplete so it triggers and pubs
func PublishChunkComplete(js jetstream.JetStream) func(service.ChunkCompleteMessage) error {
	return func(msg service.ChunkCompleteMessage) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshall chunk error: %w", err)
		}

		_, err = js.Publish(context.Background(), pubSubject, data)
		if err != nil {
			return err
		}
		return nil
	}
}
