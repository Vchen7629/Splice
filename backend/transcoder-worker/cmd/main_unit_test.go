//go:build unit

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// okJS returns a mock JetStream that succeeds through the full consumer setup.
func okJS() *test.MockJS {
	return &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{}}}
}

func TestNewLogger(t *testing.T) {
	t.Run("dev mode enables debug level", func(t *testing.T) {
		logger := newLogger(&Config{ProdMode: false})

		assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
	})

	t.Run("prod mode disables debug level", func(t *testing.T) {
		logger := newLogger(&Config{ProdMode: true})

		assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
		assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})
}

func TestRunProcessing(t *testing.T) {
	t.Run("consumer setup error returns error", func(t *testing.T) {
		js := &test.MockJS{JStreamNameErr: assert.AnError}
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)

		err := runProcessing(js, nc, silentLogger(), t.TempDir(), quit)

		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, nc.DrainCalled, "Drain should not be called if consumer setup fails")
	})

	t.Run("blocks until quit signal", func(t *testing.T) {
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runProcessing(okJS(), &test.MockDrainer{}, silentLogger(), t.TempDir(), quit)
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

	t.Run("stops consumer on quit", func(t *testing.T) {
		consumer := &test.MockConsumer{}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runProcessing(js, &test.MockDrainer{}, silentLogger(), t.TempDir(), quit))

		require.NotNil(t, consumer.Ctx)
		assert.True(t, consumer.Ctx.Stopped)
	})

	t.Run("drains NATS on quit", func(t *testing.T) {
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runProcessing(okJS(), nc, silentLogger(), t.TempDir(), quit))

		assert.True(t, nc.DrainCalled)
	})

	t.Run("drain error is returned", func(t *testing.T) {
		nc := &test.MockDrainer{DrainErr: assert.AnError}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		err := runProcessing(okJS(), nc, silentLogger(), t.TempDir(), quit)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

// writeEnvFile creates ../.env with the given content and removes it on cleanup.
// It also unsets and restores the config env vars so that values set by a
// previous test's godotenv.Load don't bleed into this one.
func writeEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "OUTPUT_DIR"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { os.Setenv(key, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
		os.Unsetenv(key)
	}
	path := filepath.Join("..", ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove(path) })
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing env file returns error", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join("..", ".env")); err == nil {
			t.Skip(".env already exists")
		}

		_, err := loadConfig()

		assert.Error(t, err)
	})

	t.Run("reads all values from env file", func(t *testing.T) {
		writeEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nOUTPUT_DIR=/custom/dir\n")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "/custom/dir", cfg.OutputDir)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		writeEnvFile(t, "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "/tmp/splice", cfg.OutputDir)
	})
}
