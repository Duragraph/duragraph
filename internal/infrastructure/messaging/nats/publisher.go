package nats

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-nats/v2/pkg/nats"
	"github.com/ThreeDotsLabs/watermill/message"
	natsgo "github.com/nats-io/nats.go"
)

// Publisher wraps Watermill NATS publisher
type Publisher struct {
	publisher *nats.Publisher
	logger    watermill.LoggerAdapter
}

// NewPublisher creates a new NATS publisher
func NewPublisher(natsURL string, logger watermill.LoggerAdapter) (*Publisher, error) {
	// Connect to NATS
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}

	// Create publisher
	pub, err := nats.NewPublisher(
		nats.PublisherConfig{
			URL:       natsURL,
			Marshaler: nats.GobMarshaler{},
		},
		logger,
	)

	if err != nil {
		return nil, err
	}

	// Ensure streams exist
	ensureStreams(js)

	return &Publisher{
		publisher: pub,
		logger:    logger,
	}, nil
}

// Publish publishes a message to a topic
func (p *Publisher) Publish(ctx context.Context, topic string, payload interface{}) error {
	// Marshal payload to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Create Watermill message
	msg := message.NewMessage(watermill.NewUUID(), data)

	// Publish
	return p.publisher.Publish(topic, msg)
}

// Close closes the publisher
func (p *Publisher) Close() error {
	return p.publisher.Close()
}

// ensureStreams creates necessary JetStream streams
func ensureStreams(js natsgo.JetStreamContext) error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{
			name:     "duragraph-events",
			subjects: []string{"duragraph.events.>"},
		},
		{
			name:     "duragraph-executions",
			subjects: []string{"duragraph.executions.>"},
		},
		{
			name:     "duragraph-runs",
			subjects: []string{"duragraph.runs.>"},
		},
	}

	for _, stream := range streams {
		// Check if stream exists
		_, err := js.StreamInfo(stream.name)
		if err == nil {
			// Stream exists
			continue
		}

		// Create stream
		_, err = js.AddStream(&natsgo.StreamConfig{
			Name:     stream.name,
			Subjects: stream.subjects,
			Storage:  natsgo.FileStorage,
			Replicas: 1,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
