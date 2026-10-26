package msgbus

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

// create the jetstream KV bucket for use in application
func CreateKV(
	bucketName string, js jetstream.JetStream, ttl time.Duration, logger *slog.Logger,
) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
		TTL:    ttl,
	})
	if err != nil {
		logger.Error("failed to create kv bucket", "bucketName", bucketName, "err", err)
		osExit(1)
		return nil
	}

	return kv
}

// connect to existing kv
func ConnectKV(js jetstream.JetStream, bucketName string, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.KeyValue(ctx, bucketName)
	if err != nil {
		logger.Error("failed to connect to kv bucket", "bucketName", bucketName, "err", err)
		osExit(1)
	}

	return kv
}

// puts a new key value pair into the jetstream kv. Note the context is hardc
func PutKeyKV(kv jetstream.KeyValue, key string, value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	return nil
}
