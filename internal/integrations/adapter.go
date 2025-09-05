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

type AuthState struct {
    Method string
    Token  string
}

type OrderBatch struct {
    Orders []Order
    Cursor string
}

type Order struct {
    ExternalRef string
    Priority    int
}

type ExternalStatus struct {
    Code string
}

type InternalEvent struct {
    Type    string
    Payload map[string]any
}

type WebhookInfo struct {
    Events []string
    Verify func(sig string, body []byte) bool
}

