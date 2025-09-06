package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gpsnav/internal/store"
)

type Publisher struct {
	Store store.Store
}

func NewPublisher(s store.Store) *Publisher {
	return &Publisher{Store: s}
}

// Emit sends an event to all subscriptions for the tenant and event type.
func (p *Publisher) Emit(ctx context.Context, tenantID, eventType string, data any) {
	subs, err := p.Store.GetSubscriptionsForEvent(ctx, tenantID, eventType)
	if err != nil || len(subs) == 0 {
		return
	}
	payload := map[string]any{
		"id":       fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		"type":     eventType,
		"tenantId": tenantID,
		"ts":       time.Now().UTC().Format(time.RFC3339),
		"data":     data,
	}
	body, _ := json.Marshal(payload)
	for _, s := range subs {
		_, _ = p.Store.EnqueueWebhook(ctx, tenantID, s.ID, eventType, s.URL, s.Secret, body)
	}
}

// SignHMAC returns hex string of HMAC-SHA256 for body
// SignHMAC is implemented in worker using the delivery secret; here we just enqueue.
