package kv

import (
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