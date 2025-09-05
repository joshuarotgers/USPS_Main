package api

import (
    "sync"
)

type SSEEvent struct {
    Type string
    Data map[string]any
}

type Broker struct {
    mu      sync.Mutex
    subs    map[string]map[chan SSEEvent]struct{} // routeId -> set of channels
}

func NewBroker() *Broker {
    return &Broker{subs: map[string]map[chan SSEEvent]struct{}{}}
}

func (b *Broker) Subscribe(routeID string) chan SSEEvent {
    ch := make(chan SSEEvent, 8)
    b.mu.Lock()
    if b.subs[routeID] == nil { b.subs[routeID] = map[chan SSEEvent]struct{}{} }
    b.subs[routeID][ch] = struct{}{}
    b.mu.Unlock()
    return ch
}

func (b *Broker) Unsubscribe(routeID string, ch chan SSEEvent) {
    b.mu.Lock()
    if m := b.subs[routeID]; m != nil {
        delete(m, ch)
        if len(m) == 0 { delete(b.subs, routeID) }
    }
    b.mu.Unlock()
    close(ch)
}

func (b *Broker) Publish(routeID string, evt SSEEvent) {
    b.mu.Lock()
    m := b.subs[routeID]
    for ch := range m {
        select { case ch <- evt: default: }
    }
    b.mu.Unlock()
}

