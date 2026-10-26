//go:build unit

package converter_test

import (
	"cmp"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/converter"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/test"
)

func TestConsumeAndProcessVideo(t *testing.T) {
	validPayload, err := json.Marshal(pipeline.VideoJobMessage{
		JobID: "job-1", StorageURL: "http://localhost:1/job-1/clip.mp4", TargetFormat: "mp4",
	})
	require.NoError(t, err)

	tests := []struct {
		name                       string
		payload                    string
		processedKV                *test.MockKV
		milestoneKV                *test.MockKV
		wantAck, wantNak, wantTerm bool
	}{
		{name: "invalid json naks", payload: "{not json", wantNak: true},
		{name: "already processed job acks", processedKV: &test.MockKV{GetFound: true}, wantAck: true},
		{name: "processed check error naks", processedKV: &test.MockKV{GetErr: errors.New("kv down")}, wantNak: true},
		{name: "cancelled job terminates", milestoneKV: &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"CANCELLED"}`)}, wantTerm: true},
		{name: "milestone update failure naks", milestoneKV: &test.MockKV{CreateErr: errors.New("kv down")}, wantNak: true},
		{name: "download failure naks", wantNak: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := &test.MockMsg{Payload: []byte(cmp.Or(tc.payload, string(validPayload)))}
			js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumerWithMsg{Msg: msg}}}

			_, err := converter.ConsumeAndProcessVideo(
				"http://localhost:1", js, cmp.Or(tc.processedKV, &test.MockKV{}), cmp.Or(tc.milestoneKV, &test.MockKV{}), test.SilentLogger(),
			)

			require.NoError(t, err)
			assert.Equal(t, tc.wantAck, msg.AckCalled, "ack")
			assert.Equal(t, tc.wantNak, msg.NakCalled, "nak")
			assert.Equal(t, tc.wantTerm, msg.TermCalled, "term")
		})
	}
}
