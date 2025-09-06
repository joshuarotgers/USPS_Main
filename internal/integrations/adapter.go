// Package integrations defines interfaces and models for external carrier/order integrations.
package integrations

// CarrierAdapter defines the minimal interface for carrier/order source integrations.
type CarrierAdapter interface {
	Name() string
	Authenticate(cfg map[string]any) (AuthState, error)
	FetchOrders(since string, cursor string) (OrderBatch, error)
	AckOrders(ids []string) error
	MapStatus(ext ExternalStatus) InternalEvent
	Webhooks() WebhookInfo
}

// AuthState describes an authentication method/state for an integration.
type AuthState struct {
	Method string
	Token  string
}

// OrderBatch contains a page of fetched orders and a cursor.
type OrderBatch struct {
	Orders []Order
	Cursor string
}

// Order is a simplified incoming order from an integration.
type Order struct {
	ExternalRef string
	Priority    int
}

// ExternalStatus is a status payload from an external system.
type ExternalStatus struct {
	Code string
}

// InternalEvent represents a mapped internal event.
type InternalEvent struct {
	Type    string
	Payload map[string]any
}

// WebhookInfo describes webhook event types and verification.
type WebhookInfo struct {
	Events []string
	Verify func(sig string, body []byte) bool
}
