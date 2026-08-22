package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// MockKV stubs jetstream.KeyValue for unit tests.
type MockKV struct {
	jetstream.KeyValue
	GetErr    error
	GetFound  bool // if true, Get returns a non-nil entry; if false, returns ErrKeyNotFound
	PutErr    error
	PutKey    string
	CreateErr error
	CreateKey string
	DeleteErr error
	DeleteKey string

	store map[string][]byte // backs Put/Get/ListKeysFiltered for real put-then-read callers
}

func (m *MockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	if value, ok := m.store[key]; ok || m.GetFound {
		return &mockKVEntry{key: key, value: value}, nil
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *MockKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	m.PutKey = key
	if m.PutErr != nil {
		return 0, m.PutErr
	}
	if m.store == nil {
		m.store = map[string][]byte{}
	}
	m.store[key] = value
	return 0, nil
}

// ListKeysFiltered ignores filters and returns every stored key; unit tests only ever
// put one job's keys per MockKV instance, so real filter matching isn't needed.
func (m *MockKV) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	keys := make(chan string, len(m.store))
	for key := range m.store {
		keys <- key
	}
	close(keys)
	return &mockKeyLister{keys: keys}, nil
}

type mockKeyLister struct{ keys chan string }

func (l *mockKeyLister) Keys() <-chan string { return l.keys }
func (l *mockKeyLister) Stop() error         { return nil }

func (m *MockKV) Create(_ context.Context, key string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	m.CreateKey = key
	if m.CreateErr != nil {
		return 0, m.CreateErr
	}
	return 0, nil
}

func (m *MockKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	m.DeleteKey = key
	return m.DeleteErr
}

type mockKVEntry struct {
	jetstream.KeyValueEntry
	key   string
	value []byte
}

func (e *mockKVEntry) Key() string   { return e.key }
func (e *mockKVEntry) Value() []byte { return e.value }

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

// MockMsg stubs jetstream.Msg for message-handling tests.
// It is kept here rather than in internal/test because it is only
// needed for subscriber behaviour and carries no value elsewhere.
type MockMsg struct {
	jetstream.Msg
	Payload   []byte
	NakCalled bool
	AckCalled bool
	NakErr    error
	AckErr    error
}

func (m *MockMsg) Data() []byte { return m.Payload }
func (m *MockMsg) Nak() error   { m.NakCalled = true; return m.NakErr }
func (m *MockMsg) Ack() error   { m.AckCalled = true; return m.AckErr }

// MockConsumer stubs jetstream.Consumer. After Consume is called, Ctx holds the
// returned ConsumeContext so tests can inspect it.
type MockConsumer struct {
	jetstream.Consumer
	ConsumeErr error
	Ctx        *MockConsumeCtx
}

func (m *MockConsumer) Consume(_ jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if m.ConsumeErr != nil {
		return nil, m.ConsumeErr
	}
	m.Ctx = &MockConsumeCtx{}
	return m.Ctx, nil
}

// MockConsumerWithMsg is like MockConsumer but delivers a single Msg to the
// handler immediately when Consume is called, useful for testing message-handling paths.
type MockConsumerWithMsg struct {
	jetstream.Consumer
	ConsumeErr error
	Msg        jetstream.Msg
	Ctx        *MockConsumeCtx
}

func (m *MockConsumerWithMsg) Consume(h jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if m.ConsumeErr != nil {
		return nil, m.ConsumeErr
	}
	m.Ctx = &MockConsumeCtx{}
	if m.Msg != nil {
		h(m.Msg)
	}
	return m.Ctx, nil
}

// MockConsumeCtx stubs jetstream.ConsumeContext.
type MockConsumeCtx struct {
	jetstream.ConsumeContext
	Stopped bool
}

func (m *MockConsumeCtx) Stop() { m.Stopped = true }

// MockDrainer stubs the ncDrainer interface used in runProcessing/runCombiner.
type MockDrainer struct {
	DrainCalled bool
	DrainErr    error
}

func (m *MockDrainer) Drain() error {
	m.DrainCalled = true
	return m.DrainErr
}
