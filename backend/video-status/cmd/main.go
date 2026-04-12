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
	"video-status/internal/handler"
	"video-status/internal/middleware"

	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Config struct {
	NatsURL  string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode bool   `envconfig:"PROD_MODE" default:"false"`
	HTTPPort string `envconfig:"HTTP_PORT" default:"8081"`
}

var osExit = os.Exit

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config values: %v", err)
	}

	logger := middleware.StructuredLogger(cfg.ProdMode)

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error("unable to connect to nats", "err", err)
		osExit(1)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("unable to connect to jetstream", "err", err)
		osExit(1)
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "job-status",
		Description: "tracks job state across the pipeline",
	})
	if err != nil {
		logger.Error("failed to create job-status kv bucket", "err", err)
		osExit(1)
	}

	advisorySub, err := handler.ListenAdvisoriesFailure(nc, js, kv, logger)
	if err != nil {
		logger.Error("failed to subscribe to advisories", "err", err)
		osExit(1)
	}

	jobCompleteSub, err := handler.ListenJobComplete(js, kv, logger)
	if err != nil {
		logger.Error("failed to subscribe to job complete stream", "err", err)
		osExit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	logger.Debug("starting service...")

	server := startHttpApi(logger, kv, cfg)

	<-quit

	err = advisorySub.Unsubscribe()
	if err != nil {
		logger.Error("failed to unsub advisory", "err", err)
	}
	jobCompleteSub.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		logger.Error("http server shutdown error", "err", err)
	}

	err = nc.Drain()
	if err != nil {
		logger.Error("nats drain error", "err", err)
	}
}

func startHttpApi(logger *slog.Logger, kv jetstream.KeyValue, cfg *Config) *http.Server {
	router := http.NewServeMux()

	jh := &handler.JobStatusHandler{Logger: logger, KV: kv}

	router.HandleFunc("GET /jobs/{id}/status", jh.PollJobStatus)

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: middleware.Cors(middleware.ApiRequestLogging(router)),
	}

	go func() {
		fmt.Printf("server running on http://localhost:%s\n", cfg.HTTPPort)

		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
			osExit(1)
		}
	}()

	return server
}

func loadConfig() (*Config, error) {
	var cfg Config

	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
