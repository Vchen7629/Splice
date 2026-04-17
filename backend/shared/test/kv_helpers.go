package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// MockKV stubs jetstream.KeyValue for unit tests.
type MockKV struct {
	jetstream.KeyValue
	GetErr   error
	GetFound bool // if true, Get returns a non-nil entry; if false, returns ErrKeyNotFound
	PutErr   error
	PutKey   string
}

func (m *MockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	if !m.GetFound {
		return nil, jetstream.ErrKeyNotFound
	}
	return &mockKVEntry{key: key}, nil
}

func (m *MockKV) Put(_ context.Context, key string, _ []byte) (uint64, error) {
	m.PutKey = key
	return 0, m.PutErr
}

type mockKVEntry struct {
	jetstream.KeyValueEntry
	key string
}

func (e *mockKVEntry) Key() string { return e.key }
