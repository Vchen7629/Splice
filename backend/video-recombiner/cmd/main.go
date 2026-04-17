package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	shanlder "shared/handler"
	"shared/kv"
	"shared/middleware"
	"shared/service"
	"shared/storage"
	"syscall"
	"video-recombiner/internal/handler"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

type Config struct {
	HTTPPort       string `envconfig:"HTTP_PORT" default:"9090"`
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode, "video-recombiner")

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

	msgRecievedKV := kv.CreateMsgProcessedKV("recombine-chunk-recieved", js, logger)
	jobStatusKV := kv.ConnectJobStatus(js, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runCombiner(js, nc, msgRecievedKV, jobStatusKV, logger, cfg.BaseStorageURL, cfg.HTTPPort, quit)
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
	msgRecievedKV, jobStatusKV jetstream.KeyValue,
	logger *slog.Logger,
	baseStorageURL, httpPort string,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service...")

	server := handler.StartHttpServer(logger, httpPort)

	consCtx, err := handler.RecombineVideo(js, msgRecievedKV, jobStatusKV, logger, baseStorageURL)
	if err != nil {
		shanlder.ShutdownHttpServer(server, logger)
		return fmt.Errorf("failed to start subscriber/publisher: %w", err)
	}

	<-quit

	shanlder.ShutdownHttpServer(server, logger)

	consCtx.Stop()
	return nc.Drain()
}
