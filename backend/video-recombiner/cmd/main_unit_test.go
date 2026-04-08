//go:build unit

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okJS returns a mock JetStream that succeeds through the full consumer setup.
func okJS() *test.MockJS {
	return &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{}}}
}

func TestStructuredLogger(t *testing.T) {
	t.Run("dev mode should enable debug level", func(t *testing.T) {
		logger := newLogger(&Config{ProdMode: false})

		assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
	})

	t.Run("prod mode should disable debug level", func(t *testing.T) {
		logger := newLogger(&Config{ProdMode: true})

		assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
		assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})
}

func TestRunCombiner(t *testing.T) {
	t.Run("consume video chunk error should return error", func(t *testing.T) {
		js := &test.MockJS{JStreamNameErr: assert.AnError}
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)

		err := runCombiner(js, nc, test.SilentLogger(), "http://storage", quit)

		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, nc.DrainCalled, "Drain should not be called if consumer setup fails")
	})

	t.Run("it should block from returning until quit signal is recieved", func(t *testing.T) {
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runCombiner(okJS(), &test.MockDrainer{}, test.SilentLogger(), "http://storage", quit)
		}()

		select {
		case <-done:
			t.Fatal("runProcessing returned before quit signal was sent")
		case <-time.After(100 * time.Millisecond):
		}

		quit <- os.Interrupt

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("runProcessing did not return after quit signal")
		}
	})

	t.Run("it should stop consumer on quit signal", func(t *testing.T) {
		consumer := &test.MockConsumer{}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runCombiner(js, &test.MockDrainer{}, test.SilentLogger(), "http://storage", quit))

		require.NotNil(t, consumer.Ctx)
		assert.True(t, consumer.Ctx.Stopped)
	})

	t.Run("it should drain nats messages on quit", func(t *testing.T) {
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runCombiner(okJS(), nc, test.SilentLogger(), "http://storage", quit))

		assert.True(t, nc.DrainCalled)
	})

	t.Run("it should handle drain errors", func(t *testing.T) {
		nc := &test.MockDrainer{DrainErr: assert.AnError}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		err := runCombiner(okJS(), nc, test.SilentLogger(), "http://storage", quit)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing env file doesnt return error", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join("..", ".env")); err == nil {
			t.Skip(".env already exists")
		}

		_, err := loadConfig()

		assert.NoError(t, err)
	})

	t.Run("reads all values from env file", func(t *testing.T) {
		test.WriteEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nBASE_STORAGE_URL=http://localhost:9333\nHTTP_PORT=9090\n")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:9333", cfg.BaseStorageURL)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		test.WriteEnvFile(t, "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:8888", cfg.BaseStorageURL)
	})
}
