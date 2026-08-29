package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splice.com/go_services/internal/shared/middleware"
	"splice.com/go_services/internal/shared/service"

	shandler "splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"
	"splice.com/go_services/internal/transcoder"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// bounds how long JetStream waits for an ack before treating chunk transcode as
	// failed and redelivering it. Must exceed worst-case transcode duration or
	// Jetstream redelivers work thats still in flight
	chunkAckWait = 10 * time.Minute
	// bounds how long a chunk claim survives before another worker retries it
	// Must stay between chunkAckWait and chunkAckWait * MaxDeliver = 3
	chunkClaimTTL = 20 * time.Minute
)

// so tests can patch this to decide when to terminate
var osExit = os.Exit

type Config struct {
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
	HTTPPort       string `envconfig:"HTTP_PORT" default:"9095"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode, "transcoder-worker")

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

	claimKV := sJetstream.CreateKV("transcode-chunk-claims", js, chunkClaimTTL, logger)
	processedKV := sJetstream.CreateKV("transcode-chunk-job-processed", js, 3*time.Hour, logger)
	jobMilestoneKV := sJetstream.ConnectKV(js, "job-milestones", logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runProcessing(cfg.BaseStorageURL, cfg.HTTPPort, processedKV, jobMilestoneKV, claimKV, js, nc, chunkAckWait, logger, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
	shandler.Publisher
}

// run the subscriber and publisher and blocks so main doesnt exit after consumevideochunk retunrs
func runProcessing(
	baseStorageURL, httpPort string,
	processedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	js jetstream.JetStream,
	nc ncDrainer,
	chunkAckWait time.Duration,
	logger *slog.Logger,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service")

	server := shandler.StartHealthHttpServer(logger, httpPort)

	consCtx, err := transcoder.ConsumeVideoChunk(baseStorageURL, js, processedKV, jobMilestoneKV, claimKV, chunkAckWait, logger)
	if err != nil {
		shandler.ShutdownHttpServer(server, logger)
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	<-quit

	shandler.ShutdownHttpServer(server, logger)

	consCtx.Stop() // stop recieving new msgs from jetstream
	return nc.Drain()
}
