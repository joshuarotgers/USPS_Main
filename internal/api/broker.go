package api

import (
	"sync"
)

// SSEEvent represents a server-sent event payload.
type SSEEvent struct {
    Type string
    Data map[string]any
}

// Broker is an in-memory pub/sub broker for route events.
type Broker struct {
    mu   sync.Mutex
    subs map[string]map[chan SSEEvent]struct{} // routeId -> set of channels
}

// NewBroker creates a new in-memory Broker.
func NewBroker() *Broker {
    return &Broker{subs: map[string]map[chan SSEEvent]struct{}{}}
}

// Subscribe registers a channel to receive events for a route.
func (b *Broker) Subscribe(routeID string) chan SSEEvent {
    ch := make(chan SSEEvent, 8)
    b.mu.Lock()
    if b.subs[routeID] == nil {
        b.subs[routeID] = map[chan SSEEvent]struct{}{}
    }
    b.subs[routeID][ch] = struct{}{}
    b.mu.Unlock()
    return ch
}

// Unsubscribe removes a subscription and closes the channel.
func (b *Broker) Unsubscribe(routeID string, ch chan SSEEvent) {
    b.mu.Lock()
    if m := b.subs[routeID]; m != nil {
        delete(m, ch)
        if len(m) == 0 {
            delete(b.subs, routeID)
        }
    }
    b.mu.Unlock()
    close(ch)
}

// Publish broadcasts an event to all subscribers of the route.
func (b *Broker) Publish(routeID string, evt SSEEvent) {
    b.mu.Lock()
    m := b.subs[routeID]
    for ch := range m {
        select {
        case ch <- evt:
        default:
        }
    }
    b.mu.Unlock()
}
