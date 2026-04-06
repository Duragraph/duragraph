package nats

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-nats/v2/pkg/nats"
	"github.com/ThreeDotsLabs/watermill/message"
)

// Subscriber wraps Watermill NATS subscriber
type Subscriber struct {
	natsURL string
	logger  watermill.LoggerAdapter
}

// NewSubscriber creates a new NATS subscriber
func NewSubscriber(natsURL, consumerGroup string, logger watermill.LoggerAdapter) (*Subscriber, error) {
	return &Subscriber{
		natsURL: natsURL,
		logger:  logger,
	}, nil
}

// Subscribe subscribes to a topic using a background context.
// The subscription lives until Close() is called.
// Deprecated: Use SubscribeWithContext for request-scoped subscriptions.
func (s *Subscriber) Subscribe(topic string) (<-chan *message.Message, error) {
	return s.SubscribeWithContext(context.Background(), topic)
}

// SubscribeWithContext subscribes to a topic with a cancellable context.
// When ctx is canceled, the subscription's output channel is closed and
// resources are released. Each call creates an independent Watermill
// subscriber so that canceling one does not affect others.
func (s *Subscriber) SubscribeWithContext(ctx context.Context, topic string) (<-chan *message.Message, error) {
	sub, err := nats.NewSubscriber(
		nats.SubscriberConfig{
			URL:         s.natsURL,
			Unmarshaler: nats.GobMarshaler{},
		},
		s.logger,
	)
	if err != nil {
		return nil, err
	}

	ch, err := sub.Subscribe(ctx, topic)
	if err != nil {
		sub.Close()
		return nil, err
	}

	go func() {
		<-ctx.Done()
		sub.Close()
	}()

	return ch, nil
}

// Close is a no-op. Individual subscriptions are managed via their contexts.
func (s *Subscriber) Close() error {
	return nil
}
