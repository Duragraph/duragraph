// Package nats — jetstream_registry.go
//
// Small concurrent-safe map for stashing in-flight jetstream.Msg
// values so the JetStreamSubscriber can route TermMessage() calls
// back to the right server-side message. Separated into its own
// file so the public-API file (jetstream_subscriber.go) stays
// readable.
package nats

import (
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

type jsRegistry struct {
	mu sync.Mutex
	m  map[string]jetstream.Msg
}

func newJSRegistry() *jsRegistry {
	return &jsRegistry{m: make(map[string]jetstream.Msg)}
}

func (r *jsRegistry) put(uuid string, msg jetstream.Msg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[uuid] = msg
}

func (r *jsRegistry) take(uuid string) (jetstream.Msg, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.m[uuid]
	if ok {
		delete(r.m, uuid)
	}
	return msg, ok
}
