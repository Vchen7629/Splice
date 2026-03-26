package main

import (
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"transcoder-worker/internal/handler"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Config struct {
	NatsURL   string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode  bool   `envconfig:"PROD_MODE" default:"false"`
	OutputDir string `envconfig:"OUTPUT_DIR" default:"/tmp/splice"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	var slogHandler slog.Handler
	if cfg.ProdMode {
		slogHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		slogHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	logger := slog.New(slogHandler).With("service", "transcoder-worker")

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error("Unable to connect to nats", "err", err)
		os.Exit(1)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("Unable to connect to jetstream", "err", err)
		os.Exit(1)
	}

	logger.Debug("starting service")

	consCtx, err := handler.ConsumeVideoChunk(js, logger, cfg.OutputDir)
	if err != nil {
		logger.Error("failed to start consumer", "err", err)
		os.Exit(1)
	}

	// blocking so main doesnt exit after consumevideochunk retunrs
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	consCtx.Stop() // stop recieving new msgs from jetstream
	nc.Drain()     // cleanup in flight and close
}

func loadConfig() (*Config, error) {
	err := godotenv.Load("../.env")
	if err != nil {
		return nil, err
	}
	var cfg Config

	err = envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
