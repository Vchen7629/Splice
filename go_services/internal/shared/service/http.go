package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var osExit = os.Exit

func newMux(reg *prometheus.Registry, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(w).Encode(map[string]string{"status": "Healthy"})
		if err != nil {
			logger.Error("failed to encode health status msg", "err", err)
		}
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return mux
}

// builds the health + metrics server and starts it
func StartHttpServer(reg *prometheus.Registry, logger *slog.Logger, httpPort string) *http.Server {
	server := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           newMux(reg, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ListenAndServeAsync(server, logger, osExit)

	return server
}

// starts provided http server in goroutine, calling exit(1) if it fails to listen
func ListenAndServeAsync(server *http.Server, logger *slog.Logger, exit func(int)) {
	go func() {
		fmt.Printf("server running on http://localhost%s\n", server.Addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
			exit(1)
		}
	}()
}

// cleanup http server when shutting down go services
func ShutdownHttpServer(server *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		logger.Error("error shutting down http server", "err", err)
	}
}
