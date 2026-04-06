package main

import (
	"fmt"
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

	logger := newLogger(cfg)

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error("unable to connect to nats", "err", err)
		os.Exit(1)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("unable to connect to jetstream", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runProcessing(js, nc, logger, cfg.OutputDir, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
}

// run the subscriber and publisher and blocks so main doesnt exit after consumevideochunk retunrs
func runProcessing(js jetstream.JetStream, nc ncDrainer, logger *slog.Logger, outputDir string, quit <-chan os.Signal) error {
	logger.Debug("starting service")

	consCtx, err := handler.ConsumeVideoChunk(js, logger, outputDir)
	if err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	<-quit

	consCtx.Stop() // stop recieving new msgs from jetstream
	return nc.Drain()
}

func newLogger(cfg *Config) *slog.Logger {
	level := slog.LevelDebug
	if cfg.ProdMode {
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	return slog.New(h).With("service", "transcoder-worker")
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
