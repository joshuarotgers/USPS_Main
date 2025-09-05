package store

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/google/uuid"
    "gpsnav/internal/model"
)

// Memory is a simple in-memory store used when no DATABASE_URL is set.
type Memory struct {
    mu     sync.Mutex
    orders map[string]model.OrderOut            // id -> order
    byTen  map[string][]string                  // tenant -> order ids
    routes map[string]model.Route               // id -> route
    hos    map[string]map[string]any            // driverId -> HOS state
    gfs    map[string]model.Geofence            // geofenceId -> geofence
    gfsTen map[string][]string                  // tenant -> geofence ids
    subs   map[string][]model.Subscription      // tenant -> subscriptions
    // Webhooks queue state
    deliveries map[string]*memDelivery          // id -> delivery state
    deliveriesByTenant map[string][]string      // tenant -> delivery ids
    dlq    []map[string]any                     // dead-lettered deliveries
    planMx map[string]map[string][]map[string]any // tenant -> planDate -> items
    optCfg map[string]map[string]any              // tenant -> config
}

func NewMemory() *Memory {
    return &Memory{
        orders: map[string]model.OrderOut{},
        byTen: map[string][]string{},
        routes: map[string]model.Route{},
        hos: map[string]map[string]any{},
        gfs: map[string]model.Geofence{},
        gfsTen: map[string][]string{},
        subs: map[string][]model.Subscription{},
        deliveries: map[string]*memDelivery{},
        deliveriesByTenant: map[string][]string{},
        dlq: []map[string]any{},
        planMx: map[string]map[string][]map[string]any{},
        optCfg: map[string]map[string]any{},
    }
}

// memDelivery augments WebhookDelivery with scheduling/metrics
type memDelivery struct {
    WebhookDelivery
    NextAttemptAt time.Time
    LastError     string
    ResponseCode  int
    LatencyMs     int
    DeliveredAt   *time.Time
}

func (m *Memory) CreateOrders(ctx context.Context, tenantID string, orders []model.OrderIn) (string, int, int, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    created := 0
    for _, o := range orders {
        id := uuid.New().String()
        m.orders[id] = model.OrderOut{ID: id, TenantID: tenantID, ExternalRef: o.ExternalRef, Priority: o.Priority, Status: "pending"}
        m.byTen[tenantID] = append(m.byTen[tenantID], id)
        created++
    }
    return "imp_mem", created, 0, nil
}

func (m *Memory) ListOrders(ctx context.Context, tenantID, status, cursor string, limit int) ([]model.OrderOut, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    ids := m.byTen[tenantID]
    start := 0
    if cursor != "" {
        for i, id := range ids {
            if id == cursor { start = i + 1; break }
        }
    }
    if limit <= 0 { limit = 100 }
    out := []model.OrderOut{}
    var next string
    for i := start; i < len(ids) && len(out) < limit; i++ {
        o := m.orders[ids[i]]
        if status == "" || o.Status == status { out = append(out, o) }
        next = ids[i]
    }
    if len(out) < limit { next = "" }
    return out, next, nil
}

func (m *Memory) GetRoute(ctx context.Context, tenantID, routeID string) (model.Route, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    r, ok := m.routes[routeID]
    if !ok { return model.Route{}, ErrNotFound }
    return r, nil
}

func (m *Memory) AssignRoute(ctx context.Context, tenantID, routeID, driverID, vehicleID string, startAt time.Time) (model.Route, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    r := m.routes[routeID]
    r.DriverID = driverID
    r.VehicleID = vehicleID
    if r.Version == 0 { r.Version = 1 }
    r.Status = "assigned"
    m.routes[routeID] = r
    return r, nil
}

func (m *Memory) PatchRoute(ctx context.Context, tenantID, routeID string, patch model.RoutePatch) (model.Route, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    r := m.routes[routeID]
    if patch.Status != "" { r.Status = patch.Status }
    if r.Version == 0 { r.Version = 1 } else { r.Version++ }
    m.routes[routeID] = r
    return r, nil
}

func (m *Memory) InsertDriverEvents(ctx context.Context, tenantID string, events []model.DriverEvent) (int, error) {
    return len(events), nil
}

func (m *Memory) CreatePoD(ctx context.Context, req model.PoDRequest) (string, string, error) {
    return uuid.New().String(), "processing", nil
}

func (m *Memory) CreateSubscription(ctx context.Context, req model.SubscriptionRequest) (model.Subscription, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    s := model.Subscription{ID: uuid.New().String(), TenantID: req.TenantID, URL: req.URL, Events: req.Events, Secret: req.Secret}
    m.subs[req.TenantID] = append(m.subs[req.TenantID], s)
    return s, nil
}

func (m *Memory) GetSubscriptionsForEvent(ctx context.Context, tenantID, eventType string) ([]model.Subscription, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    var out []model.Subscription
    for _, s := range m.subs[tenantID] {
        for _, e := range s.Events { if e == eventType { out = append(out, s); break } }
    }
    return out, nil
}

func (m *Memory) ListSubscriptions(ctx context.Context, tenantID, cursor string, limit int) ([]model.Subscription, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    list := m.subs[tenantID]
    start := 0
    if cursor != "" {
        for i := range list { if list[i].ID == cursor { start = i+1; break } }
    }
    if limit <= 0 { limit = 100 }
    end := start + limit
    if end > len(list) { end = len(list) }
    items := append([]model.Subscription(nil), list[start:end]...)
    next := ""
    if end < len(list) { next = list[end-1].ID }
    return items, next, nil
}

func (m *Memory) DeleteSubscription(ctx context.Context, tenantID, id string) error {
    m.mu.Lock(); defer m.mu.Unlock()
    arr := m.subs[tenantID]
    out := make([]model.Subscription, 0, len(arr))
    for _, s := range arr { if s.ID != id { out = append(out, s) } }
    m.subs[tenantID] = out
    return nil
}

func (m *Memory) PlanRoutes(ctx context.Context, req model.OptimizeRequest) ([]model.Route, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    id := uuid.New().String()
    // Create a tiny fake route with two legs for demo
    r := model.Route{ID: id, Version: 1, PlanDate: req.PlanDate, Status: "planned", Legs: []model.Leg{
        {ID: uuid.New().String(), Seq: 1, FromStopID: "s1", ToStopID: "s2", Status: "in_progress"},
        {ID: uuid.New().String(), Seq: 2, FromStopID: "s2", ToStopID: "s3", Status: "pending"},
    }}
    m.routes[id] = r
    // Record simple planner metrics for admin views
    algo := req.Algorithm
    if algo == "" { algo = "greedy" }
    if m.planMx[req.TenantID] == nil { m.planMx[req.TenantID] = map[string][]map[string]any{} }
    items := m.planMx[req.TenantID][req.PlanDate]
    met := map[string]any{
        "algo": algo,
        "iterations": 1,
        "improvements": 0,
        "acceptedWorse": 0,
        "bestCost": 0.0,
        "finalCost": 0.0,
        "removalSelects": []int{1, 0},
        "insertSelects": []int{1, 0},
    }
    // upsert per algo
    replaced := false
    for i := range items {
        if items[i]["algo"] == algo { items[i] = met; replaced = true; break }
    }
    if !replaced { items = append(items, met) }
    m.planMx[req.TenantID][req.PlanDate] = items
    return []model.Route{r}, "opt_mem", nil
}

func (m *Memory) AdvanceRoute(ctx context.Context, tenantID, routeID string, req model.AdvanceRequest) (model.AdvanceResponse, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    r, ok := m.routes[routeID]
    if !ok {
        return model.AdvanceResponse{}, ErrNotFound
    }
    // Policy checks
    if !req.Force && r.AutoAdvance != nil {
        pol := r.AutoAdvance
        if !pol.Enabled {
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: r}, nil
        }
        reason := req.Reason
        if reason == "arrive" { reason = "geofence_arrive" }
        if reason == "pod" { reason = "pod_ack" }
        if pol.RequirePoD && reason != "pod_ack" {
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: r}, nil
        }
        if pol.Trigger != "" && reason != "" && pol.Trigger != reason {
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: r}, nil
        }
    }
    // Find current leg (first not visited)
    idx := -1
    for i := range r.Legs {
        if r.Legs[i].Status != "visited" {
            idx = i
            break
        }
    }
    res := model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339)}
    if idx == -1 {
        return model.AdvanceResponse{Result: res, Route: r}, nil
    }
    // Mark current as visited, next as in_progress
    r.Legs[idx].Status = "visited"
    res.FromLegID = r.Legs[idx].ID
    res.FromStopID = r.Legs[idx].ToStopID
    if idx+1 < len(r.Legs) {
        r.Legs[idx+1].Status = "in_progress"
        res.ToLegID = r.Legs[idx+1].ID
        res.ToStopID = r.Legs[idx+1].ToStopID
    }
    res.Changed = true
    r.Version++
    m.routes[routeID] = r
    return model.AdvanceResponse{Result: res, Route: r}, nil
}

// HOS
func (m *Memory) UpdateHOS(ctx context.Context, tenantID, driverID string, upd model.HOSUpdate) (string, map[string]any, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    st := m.hos[driverID]
    if st == nil { st = map[string]any{"status": "off", "break": false}; m.hos[driverID] = st }
    status := st["status"].(string)
    switch upd.Action {
    case "shift_start": status = "on"; st["shiftStart"] = upd.TS
    case "shift_end": status = "off"; st["shiftEnd"] = upd.TS
    case "break_start": st["break"] = true; st["breakType"] = upd.Type; st["breakStart"] = upd.TS
    case "break_end": st["break"] = false; st["breakEnd"] = upd.TS
    }
    st["status"] = status
    if upd.Note != "" { st["note"] = upd.Note }
    return status, st, nil
}

// Geofences
func (m *Memory) CreateGeofence(ctx context.Context, tenantID string, in model.GeofenceInput) (model.Geofence, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    id := uuid.New().String()
    gf := model.Geofence{ID: id, TenantID: tenantID, Name: in.Name, Type: in.Type, RadiusM: in.RadiusM, Center: in.Center, Rules: in.Rules}
    m.gfs[id] = gf
    m.gfsTen[tenantID] = append(m.gfsTen[tenantID], id)
    return gf, nil
}

func (m *Memory) ListGeofences(ctx context.Context, tenantID, cursor string, limit int) ([]model.Geofence, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    ids := m.gfsTen[tenantID]
    start := 0
    if cursor != "" {
        for i, id := range ids {
            if id == cursor { start = i + 1; break }
        }
    }
    if limit <= 0 { limit = 100 }
    out := []model.Geofence{}
    var next string
    for i := start; i < len(ids) && len(out) < limit; i++ {
        out = append(out, m.gfs[ids[i]])
        next = ids[i]
    }
    if len(out) < limit { next = "" }
    return out, next, nil
}

func (m *Memory) GetGeofence(ctx context.Context, tenantID, id string) (model.Geofence, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    gf, ok := m.gfs[id]
    if !ok || gf.TenantID != tenantID { return model.Geofence{}, ErrNotFound }
    return gf, nil
}

func (m *Memory) PatchGeofence(ctx context.Context, tenantID, id string, in model.GeofenceInput) (model.Geofence, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    gf, ok := m.gfs[id]
    if !ok || gf.TenantID != tenantID { return model.Geofence{}, ErrNotFound }
    if in.Name != "" { gf.Name = in.Name }
    if in.Type != "" { gf.Type = in.Type }
    if in.RadiusM != 0 { gf.RadiusM = in.RadiusM }
    if in.Center != nil { gf.Center = in.Center }
    if in.Rules != nil { gf.Rules = in.Rules }
    m.gfs[id] = gf
    return gf, nil
}

func (m *Memory) DeleteGeofence(ctx context.Context, tenantID, id string) error {
    m.mu.Lock(); defer m.mu.Unlock()
    gf, ok := m.gfs[id]
    if !ok || gf.TenantID != tenantID { return ErrNotFound }
    delete(m.gfs, id)
    ids := m.gfsTen[tenantID]
    out := make([]string, 0, len(ids))
    for _, v := range ids { if v != id { out = append(out, v) } }
    m.gfsTen[tenantID] = out
    return nil
}

// Webhook deliveries
func (m *Memory) EnqueueWebhook(ctx context.Context, tenantID, subscriptionID, eventType, url, secret string, payload []byte) (string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    id := uuid.New().String()
    d := &memDelivery{WebhookDelivery: WebhookDelivery{ID: id, TenantID: tenantID, SubscriptionID: subscriptionID, EventType: eventType, URL: url, Secret: secret, Payload: payload, Status: "pending", Attempts: 0}, NextAttemptAt: time.Now()}
    m.deliveries[id] = d
    m.deliveriesByTenant[tenantID] = append(m.deliveriesByTenant[tenantID], id)
    return id, nil
}

func (m *Memory) FetchDueWebhookDeliveries(ctx context.Context, limit int) ([]WebhookDelivery, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    now := time.Now()
    out := []WebhookDelivery{}
    for _, id := range m.iterDeliveryIDs() {
        d := m.deliveries[id]
        if d == nil { continue }
        if (d.Status == "pending" || d.Status == "retry") && !d.NextAttemptAt.After(now) {
            out = append(out, d.WebhookDelivery)
            if limit > 0 && len(out) >= limit { break }
        }
    }
    return out, nil
}

func (m *Memory) MarkWebhookDelivery(ctx context.Context, id string, success bool, nextAttemptAt *time.Time, lastError string, responseCode int, latencyMs int) error {
    m.mu.Lock(); defer m.mu.Unlock()
    d := m.deliveries[id]
    if d == nil { return nil }
    d.Attempts++
    d.ResponseCode = responseCode
    d.LatencyMs = latencyMs
    if success {
        d.Status = "delivered"
        now := time.Now()
        d.DeliveredAt = &now
    } else {
        d.Status = "retry"
        d.LastError = lastError
        if nextAttemptAt != nil { d.NextAttemptAt = *nextAttemptAt } else { d.NextAttemptAt = time.Now().Add(1 * time.Minute) }
    }
    return nil
}

func (m *Memory) FailWebhookDelivery(ctx context.Context, id string, lastError string, responseCode int, latencyMs int) error {
    m.mu.Lock(); defer m.mu.Unlock()
    d := m.deliveries[id]
    if d != nil { d.Status = "failed" }
    m.dlq = append(m.dlq, map[string]any{"id": id, "lastError": lastError, "responseCode": responseCode, "latencyMs": latencyMs})
    return nil
}

func (m *Memory) ListWebhookDeliveries(ctx context.Context, tenantID, status, cursor string, limit int) ([]map[string]any, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    out := []map[string]any{}
    ids := m.deliveriesByTenant[tenantID]
    for _, id := range ids {
        d := m.deliveries[id]
        if d == nil { continue }
        if status == "" || d.Status == status {
            item := map[string]any{"id": d.ID, "eventType": d.EventType, "status": d.Status, "attempts": d.Attempts, "url": d.URL}
            if !d.NextAttemptAt.IsZero() { item["nextAttemptAt"] = d.NextAttemptAt }
            if d.LastError != "" { item["lastError"] = d.LastError }
            out = append(out, item)
        }
    }
    return out, "", nil
}

func (m *Memory) ListWebhookDLQ(ctx context.Context, tenantID, eventType string, olderThan time.Time, codeMin, codeMax int, errorQuery, cursor string, limit int) ([]map[string]any, string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    // very simple filter-less implementation for memory store
    out := append([]map[string]any(nil), m.dlq...)
    if out == nil { out = []map[string]any{} }
    return out, "", nil
}

func (m *Memory) RequeueWebhookDLQ(ctx context.Context, tenantID, id string) error {
    return nil
}

func (m *Memory) RequeueWebhookDLQBulk(ctx context.Context, tenantID string, ids []string) error { return nil }
func (m *Memory) DeleteWebhookDLQBulk(ctx context.Context, tenantID string, ids []string, olderThan time.Time) error { return nil }

func (m *Memory) SavePlanMetrics(ctx context.Context, tenantID, planDate, algo string, metrics map[string]any) error {
    m.mu.Lock(); defer m.mu.Unlock()
    if m.planMx[tenantID] == nil { m.planMx[tenantID] = map[string][]map[string]any{} }
    items := m.planMx[tenantID][planDate]
    found := false
    for i := range items { if items[i]["algo"] == algo { items[i] = metrics; items[i]["algo"] = algo; found = true; break } }
    if !found { metrics["algo"] = algo; items = append(items, metrics) }
    m.planMx[tenantID][planDate] = items
    return nil
}

func (m *Memory) ListPlanMetrics(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    items := m.planMx[tenantID][planDate]
    if algo == "" { return append([]map[string]any(nil), items...), nil }
    out := []map[string]any{}
    for _, it := range items { if it["algo"] == algo { out = append(out, it) } }
    return out, nil
}

func (m *Memory) SavePlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string, snaps []map[string]any) error { return nil }
func (m *Memory) ListPlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error) { return []map[string]any{}, nil }

func (m *Memory) GetOptimizerConfig(ctx context.Context, tenantID string) (map[string]any, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    if cfg, ok := m.optCfg[tenantID]; ok { return cfg, nil }
    return nil, nil
}

func (m *Memory) SaveOptimizerConfig(ctx context.Context, tenantID string, cfg map[string]any) error {
    m.mu.Lock(); defer m.mu.Unlock()
    m.optCfg[tenantID] = cfg
    return nil
}

func (m *Memory) RetryWebhookDelivery(ctx context.Context, tenantID, id string) error {
    m.mu.Lock(); defer m.mu.Unlock()
    d := m.deliveries[id]
    if d != nil && d.TenantID == tenantID {
        d.Status = "pending"
        d.NextAttemptAt = time.Now()
    }
    return nil
}

func (m *Memory) ListActiveRoutesForDriver(ctx context.Context, tenantID, driverID string) ([]string, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    out := []string{}
    for id, r := range m.routes {
        if r.DriverID == driverID && r.Status != "completed" { out = append(out, id) }
    }
    return out, nil
}

func (m *Memory) FindRoutesByStop(ctx context.Context, tenantID, stopID string) ([]string, error) {
    // Not tracked in memory store; return empty
    return []string{}, nil
}

func (m *Memory) RouteStats(ctx context.Context, tenantID, planDate string) (map[string]any, error) {
    m.mu.Lock(); defer m.mu.Unlock()
    // Memory store has limited info; return zeros
    return map[string]any{"routes": 0, "legs": 0, "totalDistM": 0, "totalDriveSec": 0, "avgLegsPerRoute": 0.0}, nil
}

func (m *Memory) WebhookMetrics(ctx context.Context, tenantID string, since time.Time, eventType, status string, codeMin, codeMax int, buckets []int) ([]map[string]any, error) {
    // Aggregate from in-memory deliveries
    if len(buckets) == 0 { buckets = []int{100, 500, 1000} }
    type agg struct{ cnt int; sum int; b []int }
    by := map[string]*agg{} // key: eventType|status
    add := func(typ, st string, latency int) {
        key := typ + "|" + st
        a := by[key]
        if a == nil { a = &agg{b: make([]int, len(buckets)+1)}; by[key] = a }
        a.cnt++
        if latency > 0 { a.sum += latency }
        // bucket index
        bi := len(buckets)
        for i, edge := range buckets { if latency < edge { bi = i; break } }
        a.b[bi]++
    }
    for _, ids := range m.deliveriesByTenant {
        for _, id := range ids {
            d := m.deliveries[id]
            if d == nil || d.TenantID != tenantID { continue }
            if !since.IsZero() && d.DeliveredAt != nil && d.DeliveredAt.Before(since) { continue }
            if eventType != "" && d.EventType != eventType { continue }
            st := d.Status
            if status != "" && st != status { continue }
            if codeMin > 0 && d.ResponseCode < codeMin { continue }
            if codeMax > 0 && d.ResponseCode > codeMax { continue }
            add(d.EventType, st, d.LatencyMs)
        }
    }
    // Flatten
    out := []map[string]any{}
    for k, a := range by {
        sep := -1
        for i := range k { if k[i] == '|' { sep = i; break } }
        et := k[:sep]
        st := k[sep+1:]
        avg := 0
        if a.cnt > 0 { avg = a.sum / a.cnt }
        row := map[string]any{"event_type": et, "status": st, "cnt": a.cnt, "avg_latency_ms": avg}
        for i := 0; i < len(buckets)+1; i++ { row[fmt.Sprintf("b%d", i)] = a.b[i] }
        out = append(out, row)
    }
    return out, nil
}

// helper: iterate delivery IDs by tenant order
func (m *Memory) iterDeliveryIDs() []string {
    ids := []string{}
    for _, lst := range m.deliveriesByTenant {
        ids = append(ids, lst...)
    }
    return ids
}
