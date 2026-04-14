package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/observability"
	"video-recombiner/internal/storage"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

type Config struct {
	HTTPPort	   string `envconfig:"HTTP_PORT" default:"9090"`
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := observability.StructuredLogger(cfg.ProdMode)

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

	msgRecievedKV := handler.CreateMsgRecievedKV(js, logger)
	jobStatusKV := handler.ConnectJobStatusKV(js, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	
	err = runCombiner(js, nc, msgRecievedKV, jobStatusKV, logger, cfg.BaseStorageURL, cfg.HTTPPort, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
}

func runCombiner(
	js jetstream.JetStream,
	nc ncDrainer,
	msgRecievedKV, jobStatusKV jetstream.KeyValue,
	logger *slog.Logger,
	baseStorageURL, httpPort string,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service...")

	server := startHttpServer(logger, httpPort)

	consCtx, err := handler.RecombineVideo(js, msgRecievedKV, jobStatusKV, logger, baseStorageURL)
	if err != nil {
		return fmt.Errorf("failed to start subscriber/publisher: %w", err)
	}

	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = server.Shutdown(ctx)
	if err != nil {
		logger.Error("error shutting down http server", "err", err)
	}

	consCtx.Stop()
	return nc.Drain()
}

func startHttpServer(logger *slog.Logger, httpPort string) *http.Server {
	router := http.NewServeMux()

	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "Healthy"})
	})

	server := &http.Server{
		Addr:    ":" + httpPort,
		Handler: router,
	}

	go func() {
		fmt.Printf("server running on http://localhost:%s\n", httpPort)

		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
			osExit(1)
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
