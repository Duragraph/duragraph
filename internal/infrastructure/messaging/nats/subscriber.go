package nats

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-nats/v2/pkg/nats"
	"github.com/ThreeDotsLabs/watermill/message"
)

// Subscriber wraps Watermill NATS subscriber
type Subscriber struct {
	subscriber *nats.Subscriber
	logger     watermill.LoggerAdapter
}

// NewSubscriber creates a new NATS subscriber
func NewSubscriber(natsURL, consumerGroup string, logger watermill.LoggerAdapter) (*Subscriber, error) {
	sub, err := nats.NewSubscriber(
		nats.SubscriberConfig{
			URL:         natsURL,
			Unmarshaler: nats.GobMarshaler{},
		},
		logger,
	)

	if err != nil {
		return nil, err
	}

	return &Subscriber{
		subscriber: sub,
		logger:     logger,
	}, nil
}

// Subscribe subscribes to a topic
func (s *Subscriber) Subscribe(topic string) (<-chan *message.Message, error) {
	return s.subscriber.Subscribe(context.Background(), topic)
}

// Close closes the subscriber
func (s *Subscriber) Close() error {
	return s.subscriber.Close()
}
