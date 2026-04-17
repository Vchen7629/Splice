package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// creates a durable consumer to listen to nats subject to consume messages
func CreateDurableConsumer(js jetstream.JetStream, subSubject, consName string) (jetstream.Consumer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	streamName, err := js.StreamNameBySubject(ctx, subSubject)
	if err != nil {
		return nil, fmt.Errorf("no stream found for subject: %s: %w", subSubject, err)
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          consName,
		Durable:       consName,
		FilterSubject: subSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 10, // worker wont recieve more than 10 inflight messages
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return cons, nil
}
