package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var osExit = os.Exit

// starts the http server with /health endpoint
func StartHttpServer(logger *slog.Logger, httpPort string) *http.Server {
	router := http.NewServeMux()

	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(w).Encode(map[string]string{"status": "Healthy"})
		if err != nil {
			logger.Error("failed to encode health status msg", "err", err)
		}
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

func ShutdownHttpServer(server *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		logger.Error("error shutting down http server", "err", err)
	}
}
