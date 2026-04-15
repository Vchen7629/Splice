//go:build unit

package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCombiner(t *testing.T) {
	t.Run("consumer setup error returns error", func(t *testing.T) {
		js := &test.MockJS{JStreamNameErr: assert.AnError}
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)

		err := runCombiner(js, nc, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", "0", quit)

		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, nc.DrainCalled, "Drain should not be called if consumer setup fails")
	})

	t.Run("blocks until quit signal", func(t *testing.T) {
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runCombiner(okJS(), &test.MockDrainer{}, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", "0", quit)
		}()

		select {
		case <-done:
			t.Fatal("runCombiner returned before quit signal was sent")
		case <-time.After(100 * time.Millisecond):
		}

		quit <- os.Interrupt

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("runCombiner did not return after quit signal")
		}
	})

	t.Run("stops consumer on quit", func(t *testing.T) {
		consumer := &test.MockConsumer{}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runCombiner(js, &test.MockDrainer{}, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", "0", quit))

		require.NotNil(t, consumer.Ctx)
		assert.True(t, consumer.Ctx.Stopped)
	})

	t.Run("drains NATS on quit", func(t *testing.T) {
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runCombiner(okJS(), nc, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", "0", quit))

		assert.True(t, nc.DrainCalled)
	})

	t.Run("drain error is returned", func(t *testing.T) {
		nc := &test.MockDrainer{DrainErr: assert.AnError}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		err := runCombiner(okJS(), nc, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", "0", quit)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("server shuts down when consumer setup fails", func(t *testing.T) {
		port := test.FreePort(t)
		js := &test.MockJS{JStreamNameErr: assert.AnError}
		quit := make(chan os.Signal, 1)

		runCombiner(js, &test.MockDrainer{}, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage", port, quit) //nolint:errcheck

		// If server was properly shut down, the port should be free to bind again.
		ln, err := net.Listen("tcp", ":"+port)
		require.NoError(t, err, "port should be free after server shutdown")
		ln.Close()
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
		writeEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nBASE_STORAGE_URL=http://localhost:9333\nHTTP_PORT=9090\n")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:9333", cfg.BaseStorageURL)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		writeEnvFile(t, "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:8888", cfg.BaseStorageURL)
	})
}

func TestMainFunc(t *testing.T) {
	t.Run("exits on storage health check failure", func(t *testing.T) {
		code := patchExit(t)
		writeEnvFile(t, "BASE_STORAGE_URL=http://localhost:1\n")

		main()

		assert.Equal(t, 1, *code)
	})
}
