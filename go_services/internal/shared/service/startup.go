package service

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/middleware"
	"splice.com/go_services/internal/shared/storage"
)

type BaseConfig struct {
	NatsURL        string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode       bool   `envconfig:"PROD_MODE" default:"false"`
	BaseStorageURL string `envconfig:"BASE_STORAGE_URL" default:"http://localhost:8888"`
}

// loads the service config from .env file, checks storage health (connectivity)
// before connecting to nats and jetstream
// used by both transcoder and recombiner services
func Connect(serviceName string, cfg BaseConfig) (*nats.Conn, jetstream.JetStream, *slog.Logger, error) {
	logger := middleware.StructuredLogger(cfg.ProdMode, serviceName)

	err := storage.CheckHealth(cfg.BaseStorageURL, logger)
	if err != nil {
		logger.Error("storage seedweedfs unreachable", "url", cfg.BaseStorageURL, "err", err)
		return nil, nil, logger, err
	}

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error("unable to connect to nats", "err", err)
		return nil, nil, logger, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Error("unable to connect to jetstream", "err", err)
		return nil, nil, logger, err
	}

	return nc, js, logger, nil
}

type ncDrainer interface {
	Drain() error
	handler.Publisher
}

type StartConsumer func() (jetstream.ConsumeContext, error)

// starts the service health http server, starts the processing loop
// and handles cleanup when quit is reached
func Run(logger *slog.Logger, httpPort string, nc ncDrainer, start StartConsumer, quit <-chan os.Signal) error {
	logger.Debug("starting service")

	server := handler.StartHealthHttpServer(logger, httpPort)

	consCtx, err := start()
	if err != nil {
		handler.ShutdownHttpServer(server, logger)
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	<-quit

	handler.ShutdownHttpServer(server, logger)
	consCtx.Stop() // stop recieving new msgs from jetstream
	return nc.Drain()
}
