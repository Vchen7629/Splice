package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splice.com/go_services/internal/recombiner"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/service"

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
	service.BaseConfig
	HTTPPort string `envconfig:"HTTP_PORT" default:"9090"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	nc, js, logger, err := service.Connect("video-recombiner", cfg.BaseConfig)
	if err != nil {
		osExit(1)
		return
	}

	msgRecievedKV := sJetstream.CreateKV("recombine-chunk-recieved", js, 0, logger) // no ttl for now
	jobMilestoneKV := sJetstream.ConnectKV(js, "job-milestones", logger)
	claimKV := sJetstream.CreateKV("recombine-chunk-claims", js, chunkClaimTTL, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = service.Run(logger, cfg.HTTPPort, nc, func() (jetstream.ConsumeContext, error) {
		return recombiner.RecombineVideo(js, nc, msgRecievedKV, jobMilestoneKV, claimKV, chunkAckWait, logger, cfg.BaseStorageURL)
	}, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}
