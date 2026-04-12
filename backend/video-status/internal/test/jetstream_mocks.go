package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// MockKV stubs jetstream.KeyValue for handler tests.
// Seed pre-populates entries; GetErr forces an error on every Get call.
type MockKV struct {
	jetstream.KeyValue
	GetErr  error
	entries map[string][]byte
}

func NewMockKV() *MockKV {
	return &MockKV{entries: make(map[string][]byte)}
}

// Seed stores a raw value under key so Get returns it.
func (m *MockKV) Seed(key string, value []byte) {
	m.entries[key] = value
}

func (m *MockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	v, ok := m.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return &MockKVEntry{value: v}, nil
}

// MockKVEntry stubs jetstream.KeyValueEntry.
type MockKVEntry struct {
	jetstream.KeyValueEntry
	value []byte
}

func (e *MockKVEntry) Value() []byte { return e.value }
