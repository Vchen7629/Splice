package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// cleanup http server when shutting down go services
func ShutdownHttpServer(server *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		logger.Error("error shutting down http server", "err", err)
	}
}