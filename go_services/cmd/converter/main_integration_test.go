//go:build integration

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainI(t *testing.T) {
	t.Run("exits with status 1 when service.Run fails", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		test.SetupJobMilestoneKV(t, js)
		// with no jobs stream, the consumer can't start, so Run returns an error
		require.NoError(t, js.DeleteStream(context.Background(), "jobs"))

		storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
		t.Cleanup(storage.Close)

		code := test.PatchExit(t, &osExit)
		test.WriteEnvFile(t, fmt.Sprintf("BASE_STORAGE_URL=%s\nNATS_URL=%s\nHTTP_PORT=0\n", storage.URL, nc.ConnectedUrl()))

		main()

		assert.Equal(t, 1, *code)
	})
}
