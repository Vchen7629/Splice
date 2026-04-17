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

// MockJS stubs jetstream.JetStream. Set StreamNameErr to simulate a lookup failure.
type MockJS struct {
	jetstream.JetStream
	JStreamNameErr error
	JStreamErr     error
	JStream        jetstream.Stream
}

func (m *MockJS) StreamNameBySubject(_ context.Context, _ string) (string, error) {
	return "jobs", m.JStreamNameErr
}

func (m *MockJS) Stream(_ context.Context, _ string) (jetstream.Stream, error) {
	return m.JStream, m.JStreamErr
}

// MockStream stubs jetstream.Stream.
// The consumer field is named Cons to avoid conflicting with the Consumer() method promoted by the embedded interface.
type MockStream struct {
	jetstream.Stream
	ConsumerErr error
	Cons        jetstream.Consumer
}

func (m *MockStream) CreateOrUpdateConsumer(_ context.Context, _ jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return m.Cons, m.ConsumerErr
}
