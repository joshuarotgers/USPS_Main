package store

import (
    "context"
    "errors"
    "time"

    "gpsnav/internal/model"
)

// Store is the persistence interface used by the API server.
type Store interface {
    // Orders
    CreateOrders(ctx context.Context, tenantID string, orders []model.OrderIn) (importID string, created, skipped int, err error)
    ListOrders(ctx context.Context, tenantID, status, cursor string, limit int) (items []model.OrderOut, nextCursor string, err error)

    // Routes
    GetRoute(ctx context.Context, tenantID, routeID string) (model.Route, error)
    ListRoutes(ctx context.Context, tenantID, cursor string, limit int) ([]model.Route, string, error)
    AssignRoute(ctx context.Context, tenantID, routeID, driverID, vehicleID string, startAt time.Time) (model.Route, error)
    PatchRoute(ctx context.Context, tenantID, routeID string, patch model.RoutePatch) (model.Route, error)
    PlanRoutes(ctx context.Context, req model.OptimizeRequest) (routes []model.Route, batchID string, err error)

    // Events & PoD
    InsertDriverEvents(ctx context.Context, tenantID string, events []model.DriverEvent) (accepted int, err error)
    CreatePoD(ctx context.Context, req model.PoDRequest) (podID string, status string, err error)

    // Subscriptions
    CreateSubscription(ctx context.Context, req model.SubscriptionRequest) (model.Subscription, error)
    GetSubscriptionsForEvent(ctx context.Context, tenantID, eventType string) ([]model.Subscription, error)
    ListSubscriptions(ctx context.Context, tenantID, cursor string, limit int) ([]model.Subscription, string, error)
    DeleteSubscription(ctx context.Context, tenantID, id string) error

    // Auto-advance
    AdvanceRoute(ctx context.Context, tenantID, routeID string, req model.AdvanceRequest) (model.AdvanceResponse, error)

    // HOS / Shifts
    UpdateHOS(ctx context.Context, tenantID, driverID string, upd model.HOSUpdate) (status string, hosState map[string]any, err error)

    // Geofences
    CreateGeofence(ctx context.Context, tenantID string, in model.GeofenceInput) (model.Geofence, error)
    ListGeofences(ctx context.Context, tenantID, cursor string, limit int) ([]model.Geofence, string, error)
    GetGeofence(ctx context.Context, tenantID, id string) (model.Geofence, error)
    PatchGeofence(ctx context.Context, tenantID, id string, in model.GeofenceInput) (model.Geofence, error)
    DeleteGeofence(ctx context.Context, tenantID, id string) error

    // Webhook deliveries
    EnqueueWebhook(ctx context.Context, tenantID, subscriptionID, eventType, url, secret string, payload []byte) (string, error)
    FetchDueWebhookDeliveries(ctx context.Context, limit int) ([]WebhookDelivery, error)
    MarkWebhookDelivery(ctx context.Context, id string, success bool, nextAttemptAt *time.Time, lastError string, responseCode int, latencyMs int) error
    FailWebhookDelivery(ctx context.Context, id string, lastError string, responseCode int, latencyMs int) error
    ListWebhookDeliveries(ctx context.Context, tenantID, status, cursor string, limit int) ([]map[string]any, string, error)
    RetryWebhookDelivery(ctx context.Context, tenantID, id string) error

    // Metrics
    RouteStats(ctx context.Context, tenantID, planDate string) (map[string]any, error)
    WebhookMetrics(ctx context.Context, tenantID string, since time.Time, eventType, status string, codeMin, codeMax int, buckets []int) ([]map[string]any, error)
    SavePlanMetrics(ctx context.Context, tenantID, planDate, algo string, metrics map[string]any) error
    ListPlanMetrics(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error)
    SavePlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string, snaps []map[string]any) error
    ListPlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error)

    // Optimizer config per tenant
    GetOptimizerConfig(ctx context.Context, tenantID string) (map[string]any, error)
    SaveOptimizerConfig(ctx context.Context, tenantID string, cfg map[string]any) error

    // Mapping helpers
    FindRoutesByStop(ctx context.Context, tenantID, stopID string) ([]string, error)

    // Driver → active routes mapping
    ListActiveRoutesForDriver(ctx context.Context, tenantID, driverID string) ([]string, error)

    // Dead-letter queue
    ListWebhookDLQ(ctx context.Context, tenantID, eventType string, olderThan time.Time, codeMin, codeMax int, errorQuery, cursor string, limit int) ([]map[string]any, string, error)
    RequeueWebhookDLQ(ctx context.Context, tenantID, id string) error
    RequeueWebhookDLQBulk(ctx context.Context, tenantID string, ids []string) error
    DeleteWebhookDLQBulk(ctx context.Context, tenantID string, ids []string, olderThan time.Time) error
}

var ErrNotFound = errors.New("not found")
