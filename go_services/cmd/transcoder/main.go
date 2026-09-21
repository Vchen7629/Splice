package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splice.com/go_services/internal/shared/service"

	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/transcoder"

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
	service.BaseConfig
	HTTPPort string `envconfig:"HTTP_PORT" default:"9095"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	nc, js, logger, err := service.Connect("transcoder-worker", cfg.BaseConfig)
	if err != nil {
		osExit(1)
		return
	}

	claimKV := sJetstream.CreateKV("transcode-chunk-claims", js, chunkClaimTTL, logger)
	processedKV := sJetstream.CreateKV("transcode-chunk-job-processed", js, 3*time.Hour, logger)
	jobMilestoneKV := sJetstream.ConnectKV(js, "job-milestones", logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = service.Run(logger, cfg.HTTPPort, nc, func() (jetstream.ConsumeContext, error) {
		return transcoder.ConsumeVideoChunk(cfg.BaseStorageURL, nc, js, processedKV, jobMilestoneKV, claimKV, chunkAckWait, logger)
	}, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}
