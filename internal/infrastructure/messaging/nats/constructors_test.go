package nats

import (
	"strings"
	"testing"
)

// TestNewPublisher_RejectsEmptyURL is the cheap, no-NATS smoke that
// the constructor surfaces a clear error rather than panicking inside
// nats.Connect.
func TestNewPublisher_RejectsEmptyURL(t *testing.T) {
	_, err := NewPublisher("")
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
	if !strings.Contains(err.Error(), "natsURL") {
		t.Errorf("error %q should mention 'natsURL'", err.Error())
	}
}

func TestNewSubscriber_RejectsEmptyURL(t *testing.T) {
	_, err := NewSubscriber("")
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
	if !strings.Contains(err.Error(), "natsURL") {
		t.Errorf("error %q should mention 'natsURL'", err.Error())
	}
}

// TestNewJetStreamSubscriber_RequiredFields walks each required field
// and confirms it surfaces a distinct validation error. Without these
// guards a missing FilterSubject silently subscribes to the wrong
// thing — the kind of bug that's only caught in production when no
// events show up.
func TestNewJetStreamSubscriber_RequiredFields(t *testing.T) {
	base := JetStreamSubscriberConfig{
		URL:           "nats://localhost:4222",
		StreamName:    "duragraph-events",
		FilterSubject: "duragraph.events.x.y",
		Durable:       "test-consumer",
	}

	tests := []struct {
		name      string
		mutate    func(*JetStreamSubscriberConfig)
		wantToken string
	}{
		{
			name:      "missing URL",
			mutate:    func(c *JetStreamSubscriberConfig) { c.URL = "" },
			wantToken: "URL",
		},
		{
			name:      "missing StreamName",
			mutate:    func(c *JetStreamSubscriberConfig) { c.StreamName = "" },
			wantToken: "StreamName",
		},
		{
			name:      "missing FilterSubject",
			mutate:    func(c *JetStreamSubscriberConfig) { c.FilterSubject = "" },
			wantToken: "FilterSubject",
		},
		{
			name:      "missing Durable",
			mutate:    func(c *JetStreamSubscriberConfig) { c.Durable = "" },
			wantToken: "Durable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			_, err := NewJetStreamSubscriber(cfg)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantToken) {
				t.Errorf("error %q should mention %q", err.Error(), tc.wantToken)
			}
		})
	}
}
