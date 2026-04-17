package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"shared/middleware"
	"shared/storage"
	"syscall"
	"time"
	"video-upload/internal/handler"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Config struct {
	NatsURL    string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode   bool   `envconfig:"PROD_MODE" default:"false"`
	StorageURL string `envconfig:"STORAGE_URL" default:"http://localhost:8888"`
	HTTPPort   string `envconfig:"HTTP_PORT" default:"8080"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode, "video-upload")

	err = storage.CheckHealth(cfg.StorageURL, logger)
	if err != nil {
		logger.Error("storage seedweedfs unreachable", "url", cfg.StorageURL, "err", err)
		os.Exit(1)
	}

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

	kv := handler.ConnectJobStatusKV(js, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	logger.Debug("starting service...")

	server := startHttpApi(logger, js, kv, cfg)

	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Warn("http server shutdown error", "err", err)
	}

	if err := nc.Drain(); err != nil {
		logger.Warn("nats drain error", "err", err)
	}
}

func startHttpApi(logger *slog.Logger, js jetstream.JetStream, kv jetstream.KeyValue, cfg *Config) *http.Server {
	router := http.NewServeMux()

	vh := &handler.VideoHandler{Logger: logger, JS: js, KV: kv, StorageURL: cfg.StorageURL}

	router.HandleFunc("POST /jobs/upload", vh.UploadVideo)
	router.HandleFunc("POST /jobs/download", vh.DownloadVideo)

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: middleware.Cors(middleware.ApiRequestLogging(router)),
	}

	go func() {
		fmt.Printf("server running on http://localhost:%s\n", cfg.HTTPPort)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
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
