package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/middleware"
	"video-upload/internal/service"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Config struct {
	NatsURL   string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode  bool   `envconfig:"PROD_MODE" default:"false"`
	OutputDir string `envconfig:"OUTPUT_DIR" default:"/tmp/splice"`
	HTTPPort  string `envconfig:"HTTP_PORT" default:"8080"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode)

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

	tracker, consCtx, err := handler.SubscribeJobCompletion(js, logger)
	if err != nil {
		logger.Error("failed to subscribe to job completion subject", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	logger.Debug("starting service...")

	server := startHttpApi(logger, js, tracker, cfg)

	<-quit

	consCtx.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Warn("http server shutdown error", "err", err)
	}

	if err := nc.Drain(); err != nil {
		logger.Warn("nats drain error", "err", err)
	}
}

func startHttpApi(logger *slog.Logger, js jetstream.JetStream, tracker *service.CompletedJobs, cfg *Config) *http.Server {
	router := http.NewServeMux()

	vh := &handler.VideoHandler{Logger: logger, JS: js, OutputDir: cfg.OutputDir}
	jh := &handler.JobStatusHandler{Logger: logger, Tracker: tracker}

	router.HandleFunc("POST /jobs", vh.UploadVideo)
	router.HandleFunc("GET /jobs/{id}/status", jh.PollJobStatus)
	router.HandleFunc("GET /jobs/{id}/download", vh.DownloadVideo)

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: middleware.Cors(middleware.ApiRequestLogging(router)),
	}

	go func() {
		fmt.Printf("server running on http://localhost:%s\n", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	return server
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
