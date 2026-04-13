//go:build unit

package service

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCleanUpTempFolders(t *testing.T) {
	t.Cleanup(func() { removeAll = os.RemoveAll })

	t.Run("calls removeAll for both temp dirs", func(t *testing.T) {
		var removed []string
		removeAll = func(path string) error {
			removed = append(removed, path)
			return nil
		}

		CleanUpTempFolders("job-1", silentLogger())

		assert.Contains(t, removed, "/tmp/processed_chunk-job-1")
		assert.Contains(t, removed, "/tmp/jobs/job-1")
	})

	tests := []struct {
		name       string
		failOnCall int
	}{
		{"logs warn and continues when chunk dir removal fails", 1},
		{"logs warn and continues when job dir removal fails", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			removeAll = func(_ string) error {
				calls++
				if calls == tc.failOnCall {
					return errors.New("remove failed")
				}
				return nil
			}

			CleanUpTempFolders("job-1", silentLogger())

			assert.Equal(t, 2, calls, "both dirs should be attempted even if one fails")
		})
	}
}
