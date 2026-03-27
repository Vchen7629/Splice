package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"video-upload/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const pubSubject = "jobs.video.scene-split"

func PublishVideoMetadata(js jetstream.JetStream, msg service.SceneSplitMessage) error {
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
