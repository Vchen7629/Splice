package handler

import (
	"encoding/json"
	"fmt"
	"video-recombiner/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const pubSubject = "jobs.complete"

// publish a message saying the video processing is
// finished so frontend can update
func PublishVideoProcessingComplete(
	js jetstream.JetStream,
	msg service.VideoProcessingCompleteMessage,
) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshall chunk error: %w", err)
	}

	_, err = js.PublishAsync(pubSubject, data)
	if err != nil {
		return err
	}

	return nil
}
