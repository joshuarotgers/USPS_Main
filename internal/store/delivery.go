package store

type WebhookDelivery struct {
    ID             string
    TenantID       string
    SubscriptionID string
    EventType      string
    URL            string
    Secret         string
    Payload        []byte
    Status         string
    Attempts       int
}

