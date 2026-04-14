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

	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/observability"
	"transcoder-worker/internal/storage"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// so tests can patch this to decide when to terminate
var osExit = os.Exit

type Config struct {
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
	HTTPPort	   string `envconfig:"HTTP_PORT" default:"9095"`
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

	processedKV := handler.CreateMsgProcessedKV(js, logger)
	jobStatusKV := handler.ConnectJobStatusKV(js, logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	err = runProcessing(cfg.BaseStorageURL, cfg.HTTPPort, processedKV, jobStatusKV, js, nc, logger, quit)
	if err != nil {
		logger.Error("error flushing remaining msgs", "err", err)
	}
}

type ncDrainer interface {
	Drain() error
}

// run the subscriber and publisher and blocks so main doesnt exit after consumevideochunk retunrs
func runProcessing(
	baseStorageURL, httpPort string,
	processedKV, jobStatusKV jetstream.KeyValue,
	js jetstream.JetStream,
	nc ncDrainer,
	logger *slog.Logger,
	quit <-chan os.Signal,
) error {
	logger.Debug("starting service")

	server := startHttpServer(logger, httpPort)

	consCtx, err := handler.ConsumeVideoChunk(baseStorageURL, js, processedKV, jobStatusKV, logger)
	if err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		logger.Error("error shutting down http server", "err", err)
	}

	consCtx.Stop() // stop recieving new msgs from jetstream
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
