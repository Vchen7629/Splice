package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

// handles acking with error handing
func AckWithErrHandling(logger *slog.Logger, msg jetstream.Msg) {
	err := msg.Ack()
	if err != nil {
		logger.Error("error acking msg", "err", err)
	}
}

// handles naking with error handing
func NakWithErrHandling(logger *slog.Logger, msg jetstream.Msg) {
	err := msg.Nak()
	if err != nil {
		logger.Error("error naking msg", "err", err)
	}
}

// todo: add 1 line comment
func UnmarshalJetstreamMsg[T any](msg jetstream.Msg, logger *slog.Logger) (T, bool) {
	var payload T

	err := json.Unmarshal(msg.Data(), &payload)
	if err != nil {
		logger.Error("failed to unmarshal msg from jetstream", "err", err)
		NakWithErrHandling(logger, msg)
		return payload, false
	}

	return payload, true
}

// publishes a msg to JetStream
func PublishJetstreamMsg(js jetstream.JetStream, msg any, pubSubject string) error {
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
