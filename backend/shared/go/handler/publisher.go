package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// Returns a function that publishes a msg to JetStream
func PublishJobComplete(js jetstream.JetStream, msg any, pubSubject string) error {
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
