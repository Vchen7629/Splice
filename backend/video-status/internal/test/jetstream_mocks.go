package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// MockKV stubs jetstream.KeyValue for handler tests.
// Seed pre-populates entries; GetErr forces an error on every Get call; PutErr forces an error on every Put call.
type MockKV struct {
	jetstream.KeyValue
	GetErr    error
	PutErr    error
	PutCalled bool
	entries   map[string][]byte
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

// Stubs jetstream.KeyValueEntry.
type MockKVEntry struct {
	jetstream.KeyValueEntry
	value []byte
}

func (e *MockKVEntry) Value() []byte { return e.value }

func (m *MockKV) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	m.PutCalled = true
	return 0, m.PutErr
}

// MockJS stubs jetstream.JetStream.
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
type MockStream struct {
	jetstream.Stream
	ConsumerErr error
	Cons        jetstream.Consumer
}

func (m *MockStream) CreateOrUpdateConsumer(_ context.Context, _ jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return m.Cons, m.ConsumerErr
}

// MockConsumer stubs jetstream.Consumer.
// If Msg is set it is delivered to the handler when Consume is called.
type MockConsumer struct {
	jetstream.Consumer
	ConsumeErr error
	Msg        jetstream.Msg
}

func (m *MockConsumer) Consume(h jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if m.ConsumeErr != nil {
		return nil, m.ConsumeErr
	}
	if m.Msg != nil {
		h(m.Msg)
	}
	return &MockConsumeCtx{}, nil
}

// MockConsumeCtx stubs jetstream.ConsumeContext.
type MockConsumeCtx struct {
	jetstream.ConsumeContext
	Stopped bool
}

func (m *MockConsumeCtx) Stop() { m.Stopped = true }
