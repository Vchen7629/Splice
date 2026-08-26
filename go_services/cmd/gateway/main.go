package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/gateway"
	shandler "splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/middleware"
	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/storage"
)

var natsConnect = nats.Connect

type Config struct {
	NatsURL           string `envconfig:"NATS_URL"           default:"nats://localhost:4222"`
	ProdMode          bool   `envconfig:"PROD_MODE"          default:"false"`
	StorageURL        string `envconfig:"STORAGE_URL"        default:"http://localhost:8888"`
	HTTPPort          string `envconfig:"HTTP_PORT"          default:"8080"`
	SceneDetectorURL  string `envconfig:"SCENE_DETECTOR_URL" default:"http://localhost:9098"`
	TranscoderURL     string `envconfig:"TRANSCODER_URL"     default:"http://localhost:9095"`
	RecombinerURL     string `envconfig:"RECOMBINER_URL"     default:"http://localhost:9090"`
	VideoUpscalingURL string `envconfig:"VIDEO_UPSCALING_URL" default:"http://localhost:9101"`
}

func main() {
	cfg, err := service.LoadConfig[Config]()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode, "gateway")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runGateway(cfg, logger, quit)
	if err != nil {
		logger.Error("gateway exited", "err", err)
		os.Exit(1)
	}
}

func runGateway(cfg *Config, logger *slog.Logger, quit <-chan os.Signal) error {
	err := storage.CheckHealth(cfg.StorageURL, logger)
	if err != nil {
		return fmt.Errorf("storage seedweedfs unreachable at %s: %w", cfg.StorageURL, err)
	}

	nc, err := natsConnect(cfg.NatsURL)
	if err != nil {
		return fmt.Errorf("unable to connect to nats: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("unable to connect to jetstream: %w", err)
	}

	jobMilestoneKV := sJetstream.CreateKV("job-milestones", js, 0, logger) // no ttl for now

	advisorySub, err := gateway.ListenAdvisoriesFailure(nc, js, jobMilestoneKV, logger)
	if err != nil {
		return fmt.Errorf("failed to subscribe to advisories: %w", err)
	}
	jobCompleteSub, err := gateway.ListenJobComplete(js, jobMilestoneKV, logger)
	if err != nil {
		// advisorySub is already live; drop it so a failed start leaves nothing behind.
		unsubErr := advisorySub.Unsubscribe()
		if unsubErr != nil {
			logger.Error("failed to unsub advisory during failed startup", "err", unsubErr)
		}
		return fmt.Errorf("failed to subscribe to job complete stream: %w", err)
	}

	logger.Debug("starting service...")

	server := gateway.StartHttpApi(
		logger, nc, js, jobMilestoneKV, cfg.HTTPPort, cfg.StorageURL,
		gateway.ServiceURLs{
			SceneDetector:  cfg.SceneDetectorURL,
			Transcoder:     cfg.TranscoderURL,
			Recombiner:     cfg.RecombinerURL,
			VideoUpscaling: cfg.VideoUpscalingURL,
		},
	)

	<-quit

	err = advisorySub.Unsubscribe()
	if err != nil {
		logger.Error("failed to unsub advisory", "err", err)
	}
	jobCompleteSub.Stop()

	shandler.ShutdownHttpServer(server, logger)

	return nc.Drain()
}
