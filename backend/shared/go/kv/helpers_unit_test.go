//go:build unit

package kv_test

import (
	"errors"
	"shared/kv"
	"shared/test"
	"testing"
)

func TestAckWithErrHandling(t *testing.T) {
	t.Run("calls Ack on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		kv.AckWithErrHandling(test.SilentLogger(), msg)

		if !msg.AckCalled {
			t.Error("expected Ack to be called")
		}
	})

	t.Run("logs error when Ack fails", func(t *testing.T) {
		msg := &test.MockMsg{AckErr: errors.New("ack failed")}

		// Should not panic even when Ack returns an error
		kv.AckWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestNakWithErrHandling(t *testing.T) {
	t.Run("calls Nak on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		kv.NakWithErrHandling(test.SilentLogger(), msg)

		if !msg.NakCalled {
			t.Error("expected Nak to be called")
		}
	})

	t.Run("logs error when Nak fails", func(t *testing.T) {
		msg := &test.MockMsg{NakErr: errors.New("nak failed")}

		// Should not panic even when Nak returns an error
		kv.NakWithErrHandling(test.SilentLogger(), msg)
	})
}
