package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splice.com/go_services/internal/recombiner"
	shandler "splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/middleware"
	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NOTE: in the future might need to add a keepALive ping by running it in msg.InPRogress so we dont have the guess ttl
const (
	// bounds how long JetStream waits for an ack before treating chunk transcode as
	// failed and redelivering it. Must exceed worst-case transcode duration or
	// Jetstream redelivers work thats still in flight
	chunkAckWait = 5 * time.Minute
	// bounds how long a chunk claim survives before another worker retries it
	// Must stay between chunkAckWait and chunkAckWait * MaxDeliver = 3
	chunkClaimTTL = 10 * time.Minute
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

	msgRecievedKV := sJetstream.CreateKV("recombine-chunk-recieved", js, 0, logger) // no ttl for now
	jobMilestoneKV := sJetstream.ConnectKV(js, "job-milestones", logger)
	claimKV := sJetstream.CreateKV("recombine-chunk-claims", js, chunkClaimTTL, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runCombiner(js, nc, msgRecievedKV, jobMilestoneKV, claimKV, logger, cfg.BaseStorageURL, cfg.HTTPPort, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
	shandler.Publisher
}

func runCombiner(
	js jetstream.JetStream,
	nc ncDrainer,
	msgRecievedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	logger *slog.Logger,
	baseStorageURL, httpPort string,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service...")

	server := shandler.StartHealthHttpServer(logger, httpPort)

	consCtx, err := recombiner.RecombineVideo(js, nc, msgRecievedKV, jobMilestoneKV, claimKV, chunkAckWait, logger, baseStorageURL)
	if err != nil {
		shandler.ShutdownHttpServer(server, logger)
		return fmt.Errorf("failed to start subscriber/publisher: %w", err)
	}

	<-quit

	shandler.ShutdownHttpServer(server, logger)

	consCtx.Stop()
	return nc.Drain()
}
