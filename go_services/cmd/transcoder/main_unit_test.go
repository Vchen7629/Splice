//go:build unit

package main

import (
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
)

func TestMainFunc(t *testing.T) {
	t.Run("exits on storage health check failure", func(t *testing.T) {
		code := test.PatchExit(t, &osExit)
		test.WriteEnvFile(t, "BASE_STORAGE_URL=http://localhost:1\n")

		main()

		assert.Equal(t, 1, *code)
	})
}
