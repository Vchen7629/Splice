//go:build unit

package main

import (
	"net"
	"os"
	"strconv"
	"testing"
	"time"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okJS returns a mock JetStream that succeeds through the full consumer setup.
func okJS() *test.MockJS {
	return &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{}}}
}

func TestRunProcessing(t *testing.T) {
	t.Run("consumer setup error returns error", func(t *testing.T) {
		js := &test.MockJS{JStreamNameErr: assert.AnError}
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)

		err := runProcessing("http://storage", "0", &test.MockKV{}, &test.MockKV{}, js, nc, test.SilentLogger(), quit)

		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, nc.DrainCalled, "Drain should not be called if consumer setup fails")
	})

	t.Run("blocks until quit signal", func(t *testing.T) {
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runProcessing("http://storage", "0", &test.MockKV{}, &test.MockKV{}, okJS(), &test.MockDrainer{}, test.SilentLogger(), quit)
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

		require.NoError(t, runProcessing("http://storage", "0", &test.MockKV{}, &test.MockKV{}, js, &test.MockDrainer{}, test.SilentLogger(), quit))

		require.NotNil(t, consumer.Ctx)
		assert.True(t, consumer.Ctx.Stopped)
	})

	t.Run("drains NATS on quit", func(t *testing.T) {
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, runProcessing("http://storage", "0", &test.MockKV{}, &test.MockKV{}, okJS(), nc, test.SilentLogger(), quit))

		assert.True(t, nc.DrainCalled)
	})

	t.Run("server shuts down when consumer setup fails", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
		ln.Close()

		js := &test.MockJS{JStreamNameErr: assert.AnError}
		quit := make(chan os.Signal, 1)

		runProcessing("http://storage", port, &test.MockKV{}, &test.MockKV{}, js, &test.MockDrainer{}, test.SilentLogger(), quit) //nolint:errcheck

		// If server was properly shut down, the port should be free to bind again.
		ln2, err := net.Listen("tcp", ":"+port)
		require.NoError(t, err, "port should be free after server shutdown")
		ln2.Close()
	})

	t.Run("drain error is returned", func(t *testing.T) {
		nc := &test.MockDrainer{DrainErr: assert.AnError}
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		err := runProcessing("http://storage", "0", &test.MockKV{}, &test.MockKV{}, okJS(), nc, test.SilentLogger(), quit)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestMainFunc(t *testing.T) {
	t.Run("exits on storage health check failure", func(t *testing.T) {
		code := patchOsExit(t)
		writeEnvFile(t, "BASE_STORAGE_URL=http://localhost:1\n")

		main()

		assert.Equal(t, 1, *code)
	})
}
