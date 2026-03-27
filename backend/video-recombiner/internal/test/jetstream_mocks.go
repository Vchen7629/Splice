package test

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

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

func (m *MockStream) Consumer(_ context.Context, _ string) (jetstream.Consumer, error) {
	panic("MockStream.Consumer not implemented")
}

func (m *MockStream) CreateOrUpdateConsumer(_ context.Context, _ jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return m.Cons, m.ConsumerErr
}

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

// MockDrainer stubs the ncDrainer interface used in runProcessing.
type MockDrainer struct {
	DrainCalled bool
	DrainErr    error
}

func (m *MockDrainer) Drain() error {
	m.DrainCalled = true
	return m.DrainErr
}
