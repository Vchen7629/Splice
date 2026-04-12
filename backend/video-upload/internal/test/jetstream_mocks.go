package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// MockJS stubs jetstream.JetStream.
type MockJS struct {
	jetstream.JetStream
	JStreamNameErr error
	JStreamErr     error
	JStream        jetstream.Stream
	PublishErr     error
	PublishCalled  bool
}

func (m *MockJS) StreamNameBySubject(_ context.Context, _ string) (string, error) {
	return "jobs", m.JStreamNameErr
}

func (m *MockJS) Stream(_ context.Context, _ string) (jetstream.Stream, error) {
	return m.JStream, m.JStreamErr
}

func (m *MockJS) Publish(_ context.Context, _ string, _ []byte, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	m.PublishCalled = true
	return nil, m.PublishErr
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

// MockConsumer stubs jetstream.Consumer. If Msg is set, it is delivered to the handler when Consume is called.
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

// MockKV stubs jetstream.KeyValue. PutErr is returned by Put if set.
type MockKV struct {
	jetstream.KeyValue
	PutErr    error
	PutCalled bool
}

func (m *MockKV) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	m.PutCalled = true
	return 0, m.PutErr
}
