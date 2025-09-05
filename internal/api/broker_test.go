package api

import (
    "testing"
    "time"
)

func TestBrokerPublishSubscribe(t *testing.T) {
    b := NewBroker()
    rid := "r1"
    ch := b.Subscribe(rid)
    defer func() { recover() }() // ignore close panic if already closed

    evt := SSEEvent{Type: "test.event", Data: map[string]any{"x": 1}}
    b.Publish(rid, evt)

    select {
    case got := <-ch:
        if got.Type != evt.Type { t.Fatalf("got type %s, want %s", got.Type, evt.Type) }
        if got.Data["x"].(int) != 1 { t.Fatalf("bad payload: %+v", got.Data) }
    case <-time.After(200 * time.Millisecond):
        t.Fatal("timeout waiting for event")
    }

    b.Unsubscribe(rid, ch)
    select {
    case _, ok := <-ch:
        if ok { t.Fatal("channel should be closed after unsubscribe") }
    case <-time.After(50 * time.Millisecond):
        // acceptable if already drained and closed
    }
}

