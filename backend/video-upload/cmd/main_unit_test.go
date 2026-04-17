//go:build unit

package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMainFunc(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "exits when storage is unreachable",
			setup: func(t *testing.T) {
				writeEnvFile(t, "STORAGE_URL=http://localhost:1\n")
			},
		},
		{
			name: "exits when nats is unreachable",
			setup: func(t *testing.T) {
				writeEnvFile(t, "STORAGE_URL="+fakeStorageServer(t)+"\nNATS_URL=nats://localhost:1\n")
				patchNatsConnect(t, errors.New("nats unreachable"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := patchOsExit(t)
			tc.setup(t)

			main()

			assert.Equal(t, 1, *code)
		})
	}
}
