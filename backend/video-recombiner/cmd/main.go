package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/observability"
	"video-recombiner/internal/storage"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

type Config struct {
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := observability.StructuredLogger(cfg.ProdMode)

	err = storage.CheckHealth(cfg.BaseStorageURL, logger)
	if err != nil {
		logger.Error("storage seedweedfs unreachable", "url", cfg.BaseStorageURL, "err", err)
		osExit(1)
		return
	}

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error("unable to connect to nats", "err", err)
		osExit(1)
		return
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("unable to connect to jetstream", "err", err)
		osExit(1)
		return
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "recombine-chunk-recieved",
		Description: "tracks video chunk for the jobID is already recieved for idempotency",
		TTL:         3 * time.Hour,
	})
	if err != nil {
		logger.Error("failed to create recombine-chunk-recieved kv bucket", "err", err)
		osExit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runCombiner(js, nc, kv, logger, cfg.BaseStorageURL, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
}

func runCombiner(
	js jetstream.JetStream,
	nc ncDrainer,
	kv jetstream.KeyValue,
	logger *slog.Logger,
	baseStorageURL string,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service...")

	consCtx, err := handler.RecombineVideo(js, kv, logger, baseStorageURL)
	if err != nil {
		return fmt.Errorf("failed to start subscriber/publisher: %w", err)
	}

	<-quit

	consCtx.Stop()
	return nc.Drain()
}

func loadConfig() (*Config, error) {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("missing .env file")
	}
	var cfg Config

	err = envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
