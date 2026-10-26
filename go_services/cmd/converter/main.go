package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splice.com/go_services/internal/converter"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/service"

	"github.com/nats-io/nats.go/jetstream"
)

// so tests can patch this to decide when to terminate
var osExit = os.Exit

type Config struct {
	service.BaseConfig
	HTTPPort string `envconfig:"HTTP_PORT" default:"9099"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := service.StructuredLogger(cfg.ProdMode, "format-converter")

	nc, js, err := service.Connect(logger, cfg.BaseConfig)
	if err != nil {
		osExit(1)
		return
	}

	processedKV := msgbus.CreateKV("converter-job-processed", js, 3*time.Hour, logger)
	jobMilestoneKV := msgbus.ConnectKV(js, "job-milestones", logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	if err := service.Run(logger, cfg.HTTPPort, nc, func() (jetstream.ConsumeContext, error) {
		return converter.ConsumeAndProcessVideo(cfg.BaseStorageURL, js, processedKV, jobMilestoneKV, logger)
	}, quit); err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
		osExit(1)
	}
}
