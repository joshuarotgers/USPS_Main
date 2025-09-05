package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    _ "github.com/jackc/pgx/v5/stdlib"
    "github.com/google/uuid"
    "encoding/json"
    "math"
    "strings"
    "crypto/sha256"
    "encoding/hex"

    "gpsnav/internal/model"
    "gpsnav/internal/opt"
)

type Postgres struct {
    db *sql.DB
}

func NewPostgres(dsn string) (*Postgres, error) {
    db, err := sql.Open("pgx", dsn)
    if err != nil {
        return nil, err
    }
    if err := db.Ping(); err != nil {
        return nil, err
    }
    return &Postgres{db: db}, nil
}

// CreateOrders inserts orders and their stops. Dedup by (tenant_id, external_ref).
func (p *Postgres) CreateOrders(ctx context.Context, tenantID string, orders []model.OrderIn) (string, int, int, error) {
    importID := fmt.Sprintf("imp_%d", time.Now().UnixNano())
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil { return "", 0, 0, err }
    defer func(){ _ = tx.Rollback() }()

    created := 0
    skipped := 0
    for _, o := range orders {
        oid := uuid.New()
        // Upsert by (tenant_id, external_ref) if provided
        if o.ExternalRef != "" {
            // check unique constraint
            var existsID string
            err = tx.QueryRowContext(ctx, `SELECT id::text FROM orders WHERE tenant_id=$1 AND external_ref=$2`, tenantID, o.ExternalRef).Scan(&existsID)
            if err == nil {
                skipped++
                continue
            }
            if err != nil && !errors.Is(err, sql.ErrNoRows) {
                return "", 0, 0, err
            }
        }
        _, err = tx.ExecContext(ctx, `INSERT INTO orders (id, tenant_id, external_ref, priority, status, attrs) VALUES ($1,$2,$3,$4,$5,$6)`,
            oid, tenantID, nullIfEmpty(o.ExternalRef), o.Priority, "pending", toJSON(o.Attributes))
        if err != nil { return "", 0, 0, err }
        // stops
        seq := 0
        for _, s := range o.Stops {
            sid := uuid.New()
            var tw any
            if s.TimeWindow != nil && s.TimeWindow.Start != "" && s.TimeWindow.End != "" {
                tw = fmt.Sprintf("[%s,%s]", s.TimeWindow.Start, s.TimeWindow.End)
            } else {
                tw = nil
            }
            var lat, lng any
            if s.Location != nil {
                lat = s.Location.Lat
                lng = s.Location.Lng
            }
            _, err = tx.ExecContext(ctx, `INSERT INTO stops (id, tenant_id, order_id, type, address, lat, lng, time_window, service_time_sec, required_skills, status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
                sid, tenantID, oid, s.Type, nullIfEmpty(s.Address), lat, lng, tw, s.ServiceTimeSec, pqStringArray(s.RequiredSkills), "pending")
            if err != nil { return "", 0, 0, err }
            seq++
        }
        created++
    }
    if err := tx.Commit(); err != nil { return "", 0, 0, err }
    return importID, created, skipped, nil
}

func (p *Postgres) ListOrders(ctx context.Context, tenantID, status, cursor string, limit int) ([]model.OrderOut, string, error) {
    if limit <= 0 || limit > 500 { limit = 100 }
    // Simple offset cursor (opaque in real impl). For now cursor is last id text.
    var rows *sql.Rows
    var err error
    if status != "" {
        if cursor != "" {
            rows, err = p.db.QueryContext(ctx, `SELECT id::text, external_ref, priority, status FROM orders WHERE tenant_id=$1 AND status=$2 AND id::text > $3 ORDER BY id LIMIT $4`, tenantID, status, cursor, limit)
        } else {
            rows, err = p.db.QueryContext(ctx, `SELECT id::text, external_ref, priority, status FROM orders WHERE tenant_id=$1 AND status=$2 ORDER BY id LIMIT $3`, tenantID, status, limit)
        }
    } else {
        if cursor != "" {
            rows, err = p.db.QueryContext(ctx, `SELECT id::text, external_ref, priority, status FROM orders WHERE tenant_id=$1 AND id::text > $2 ORDER BY id LIMIT $3`, tenantID, cursor, limit)
        } else {
            rows, err = p.db.QueryContext(ctx, `SELECT id::text, external_ref, priority, status FROM orders WHERE tenant_id=$1 ORDER BY id LIMIT $2`, tenantID, limit)
        }
    }
    if err != nil { return nil, "", err }
    defer rows.Close()
    out := []model.OrderOut{}
    var last string
    for rows.Next() {
        var o model.OrderOut
        var ext sql.NullString
        if err := rows.Scan(&o.ID, &ext, &o.Priority, &o.Status); err != nil { return nil, "", err }
        o.ExternalRef = ext.String
        out = append(out, o)
        last = o.ID
    }
    var next string
    if len(out) == limit { next = last }
    return out, next, nil
}

func (p *Postgres) GetRoute(ctx context.Context, tenantID, routeID string) (model.Route, error) {
    var r model.Route
    row := p.db.QueryRowContext(ctx, `SELECT id::text, version, plan_date, status, driver_id::text, vehicle_id::text, auto_advance FROM routes WHERE tenant_id=$1 AND id=$2`, tenantID, routeID)
    var driverID, vehicleID sql.NullString
    var aa any
    if err := row.Scan(&r.ID, &r.Version, &r.PlanDate, &r.Status, &driverID, &vehicleID, &aa); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return r, ErrNotFound }
        return r, err
    }
    r.DriverID = driverID.String
    // decode auto_advance
    switch v := aa.(type) {
    case []byte:
        if len(v) > 0 { var pol model.AutoAdvancePolicy; _ = json.Unmarshal(v, &pol); r.AutoAdvance = &pol }
    case string:
        if v != "" { var pol model.AutoAdvancePolicy; _ = json.Unmarshal([]byte(v), &pol); r.AutoAdvance = &pol }
    }
    r.VehicleID = vehicleID.String
    legsRows, err := p.db.QueryContext(ctx, `SELECT id::text, seq, kind, break_sec, from_stop_id::text, to_stop_id::text, dist_m, drive_sec, eta_arrival, eta_departure, status FROM route_legs WHERE tenant_id=$1 AND route_id=$2 ORDER BY seq`, tenantID, routeID)
    if err != nil { return r, err }
    defer legsRows.Close()
    for legsRows.Next() {
        var l model.Leg
        var kind sql.NullString
        var breakSec sql.NullInt64
        var fromID, toID sql.NullString
        var etaA, etaD sql.NullTime
        if err := legsRows.Scan(&l.ID, &l.Seq, &kind, &breakSec, &fromID, &toID, &l.DistM, &l.DriveSec, &etaA, &etaD, &l.Status); err != nil { return r, err }
        if kind.Valid { l.Kind = kind.String }
        if breakSec.Valid { l.BreakSec = int(breakSec.Int64) }
        l.FromStopID = fromID.String
        l.ToStopID = toID.String
        if etaA.Valid { l.ETAArrival = etaA.Time.Format(time.RFC3339) }
        if etaD.Valid { l.ETADeparture = etaD.Time.Format(time.RFC3339) }
        r.Legs = append(r.Legs, l)
    }
    // compute summary
    breaks := 0
    breakSecTotal := 0
    for _, lg := range r.Legs { if strings.ToLower(lg.Kind) == "break" { breaks++; breakSecTotal += lg.BreakSec } }
    r.BreaksCount = breaks
    r.TotalBreakSec = breakSecTotal
    return r, nil
}

func (p *Postgres) AssignRoute(ctx context.Context, tenantID, routeID, driverID, vehicleID string, startAt time.Time) (model.Route, error) {
    _, err := p.db.ExecContext(ctx, `UPDATE routes SET driver_id=$1, vehicle_id=$2 WHERE tenant_id=$3 AND id=$4`, driverID, vehicleID, tenantID, routeID)
    if err != nil { return model.Route{}, err }
    return p.GetRoute(ctx, tenantID, routeID)
}

func (p *Postgres) PatchRoute(ctx context.Context, tenantID, routeID string, patch model.RoutePatch) (model.Route, error) {
    if patch.Status == "" && patch.AutoAdvance == nil {
        return p.GetRoute(ctx, tenantID, routeID)
    }
    if patch.Status != "" && patch.AutoAdvance != nil {
        _, err := p.db.ExecContext(ctx, `UPDATE routes SET status=$1, auto_advance=$2 WHERE tenant_id=$3 AND id=$4`, patch.Status, patch.AutoAdvance, tenantID, routeID)
        if err != nil { return model.Route{}, err }
    } else if patch.Status != "" {
        _, err := p.db.ExecContext(ctx, `UPDATE routes SET status=$1 WHERE tenant_id=$2 AND id=$3`, patch.Status, tenantID, routeID)
        if err != nil { return model.Route{}, err }
    } else if patch.AutoAdvance != nil {
        _, err := p.db.ExecContext(ctx, `UPDATE routes SET auto_advance=$1 WHERE tenant_id=$2 AND id=$3`, patch.AutoAdvance, tenantID, routeID)
        if err != nil { return model.Route{}, err }
    }
    return p.GetRoute(ctx, tenantID, routeID)
}

func (p *Postgres) InsertDriverEvents(ctx context.Context, tenantID string, events []model.DriverEvent) (int, error) {
    if len(events) == 0 { return 0, nil }
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil { return 0, err }
    defer func(){ _ = tx.Rollback() }()
    for _, e := range events {
        // augment payload with routeId/stopId/legId for policy queries
        payload := map[string]any{}
        for k, v := range e.Payload { payload[k] = v }
        if e.RouteID != "" { payload["routeId"] = e.RouteID }
        if e.StopID != "" { payload["stopId"] = e.StopID }
        if e.LegID != "" { payload["legId"] = e.LegID }
        _, err := tx.ExecContext(ctx, `INSERT INTO events (id, tenant_id, type, entity_id, ts, payload, source) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
            uuid.New(), tenantID, e.Type, nullIfEmpty(e.RouteID), e.TS, toJSON(payload), "driver")
        if err != nil { return 0, err }
    }
    if err := tx.Commit(); err != nil { return 0, err }
    return len(events), nil
}

func (p *Postgres) CreatePoD(ctx context.Context, req model.PoDRequest) (string, string, error) {
    id := uuid.New().String()
    _, err := p.db.ExecContext(ctx, `INSERT INTO pods (id, tenant_id, order_id, stop_id, type, media_url, hash, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
        id, req.TenantID, req.OrderID, req.StopID, req.Type, mediaURL(req.Media), mediaHash(req.Media), toJSON(req.Metadata))
    if err != nil { return "", "", err }
    return id, "processing", nil
}

func (p *Postgres) CreateSubscription(ctx context.Context, req model.SubscriptionRequest) (model.Subscription, error) {
    id := uuid.New().String()
    ev, _ := json.Marshal(req.Events)
    _, err := p.db.ExecContext(ctx, `INSERT INTO subscriptions (id, tenant_id, url, events, secret) VALUES ($1,$2,$3,$4,$5)`, id, req.TenantID, req.URL, ev, req.Secret)
    if err != nil { return model.Subscription{}, err }
    return model.Subscription{ID: id, TenantID: req.TenantID, URL: req.URL, Events: req.Events, Secret: req.Secret}, nil
}

func (p *Postgres) GetSubscriptionsForEvent(ctx context.Context, tenantID, eventType string) ([]model.Subscription, error) {
    rows, err := p.db.QueryContext(ctx, `SELECT id::text, url, secret, events FROM subscriptions WHERE tenant_id=$1 AND events @> $2::jsonb`, tenantID, fmt.Sprintf("[\"%s\"]", eventType))
    if err != nil { return nil, err }
    defer rows.Close()
    out := []model.Subscription{}
    for rows.Next() {
        var s model.Subscription
        var events any
        if err := rows.Scan(&s.ID, &s.URL, &s.Secret, &events); err != nil { return nil, err }
        s.TenantID = tenantID
        if b, ok := events.([]byte); ok { _ = json.Unmarshal(b, &s.Events) }
        out = append(out, s)
    }
    return out, nil
}

func (p *Postgres) ListSubscriptions(ctx context.Context, tenantID, cursor string, limit int) ([]model.Subscription, string, error) {
    if limit <= 0 || limit > 500 { limit = 100 }
    var rows *sql.Rows
    var err error
    if cursor != "" {
        rows, err = p.db.QueryContext(ctx, `SELECT id::text, url, secret, events FROM subscriptions WHERE tenant_id=$1 AND id::text > $2 ORDER BY id LIMIT $3`, tenantID, cursor, limit)
    } else {
        rows, err = p.db.QueryContext(ctx, `SELECT id::text, url, secret, events FROM subscriptions WHERE tenant_id=$1 ORDER BY id LIMIT $2`, tenantID, limit)
    }
    if err != nil { return nil, "", err }
    defer rows.Close()
    var out []model.Subscription
    var last string
    for rows.Next() {
        var s model.Subscription
        var ev []byte
        if err := rows.Scan(&s.ID, &s.URL, &s.Secret, &ev); err != nil { return nil, "", err }
        s.TenantID = tenantID
        _ = json.Unmarshal(ev, &s.Events)
        out = append(out, s)
        last = s.ID
    }
    next := ""
    if len(out) == limit { next = last }
    return out, next, nil
}

func (p *Postgres) DeleteSubscription(ctx context.Context, tenantID, id string) error {
    _, err := p.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE tenant_id=$1 AND id=$2`, tenantID, id)
    return err
}

// Webhook deliveries
func (p *Postgres) EnqueueWebhook(ctx context.Context, tenantID, subscriptionID, eventType, url, secret string, payload []byte) (string, error) {
    id := uuid.New().String()
    dk := computeDedupKey(payload)
    _, err := p.db.ExecContext(ctx, `INSERT INTO webhook_deliveries (id, tenant_id, subscription_id, event_type, url, secret, payload, status, attempts, next_attempt_at, dedup_key)
        VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',0,now(),$8)
        ON CONFLICT (tenant_id, event_type, url, dedup_key) DO NOTHING`, id, tenantID, nullIfEmpty(subscriptionID), eventType, url, nullIfEmpty(secret), payload, dk)
    if err != nil { return "", err }
    return id, nil
}

func (p *Postgres) FetchDueWebhookDeliveries(ctx context.Context, limit int) ([]WebhookDelivery, error) {
    rows, err := p.db.QueryContext(ctx, `SELECT id::text, tenant_id::text, COALESCE(subscription_id::text,''), event_type, url, COALESCE(secret,''), payload, status, attempts 
        FROM webhook_deliveries WHERE status IN ('pending','retry') AND next_attempt_at <= now() ORDER BY next_attempt_at ASC LIMIT $1`, limit)
    if err != nil { return nil, err }
    defer rows.Close()
    out := []WebhookDelivery{}
    for rows.Next() {
        var d WebhookDelivery
        var payload []byte
        if err := rows.Scan(&d.ID, &d.TenantID, &d.SubscriptionID, &d.EventType, &d.URL, &d.Secret, &payload, &d.Status, &d.Attempts); err != nil { return nil, err }
        d.Payload = payload
        out = append(out, d)
    }
    return out, nil
}

func (p *Postgres) MarkWebhookDelivery(ctx context.Context, id string, success bool, nextAttemptAt *time.Time, lastError string, responseCode int, latencyMs int) error {
    status := "delivered"
    if !success {
        status = "retry"
        if nextAttemptAt == nil { t := time.Now().Add(1 * time.Minute); nextAttemptAt = &t }
        _, err := p.db.ExecContext(ctx, `UPDATE webhook_deliveries SET attempts=attempts+1, status=$1, last_error=$2, next_attempt_at=$3, updated_at=now(), response_code=$5, latency_ms=$6 WHERE id=$4`, status, nullIfEmpty(lastError), *nextAttemptAt, id, responseCode, latencyMs)
        return err
    }
    _, err := p.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status='delivered', delivered_at=now(), updated_at=now(), response_code=$2, latency_ms=$3 WHERE id=$1`, id, responseCode, latencyMs)
    return err
}

func (p *Postgres) FailWebhookDelivery(ctx context.Context, id string, lastError string, responseCode int, latencyMs int) error {
    _, err := p.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status='failed', last_error=$2, updated_at=now(), response_code=$3, latency_ms=$4 WHERE id=$1`, id, nullIfEmpty(lastError), responseCode, latencyMs)
    if err != nil { return err }
    // move to DLQ
    _, err = p.db.ExecContext(ctx, `INSERT INTO webhook_dlq (id, tenant_id, delivery_id, event_type, url, secret, payload, attempts, last_error)
        SELECT gen_random_uuid(), tenant_id, id, event_type, url, secret, payload, attempts+1, $2 FROM webhook_deliveries WHERE id=$1`, id, nullIfEmpty(lastError))
    return err
}

func (p *Postgres) ListWebhookDeliveries(ctx context.Context, tenantID, status, cursor string, limit int) ([]map[string]any, string, error) {
    if limit <= 0 || limit > 500 { limit = 100 }
    q := `SELECT id::text, event_type, status, attempts, next_attempt_at, last_error, url FROM webhook_deliveries WHERE tenant_id=$1`
    var rows *sql.Rows
    var err error
    if status != "" {
        q += ` AND status=$2 ORDER BY id LIMIT $3`
        rows, err = p.db.QueryContext(ctx, q, tenantID, status, limit)
    } else {
        q += ` ORDER BY id LIMIT $2`
        rows, err = p.db.QueryContext(ctx, q, tenantID, limit)
    }
    if err != nil { return nil, "", err }
    defer rows.Close()
    out := []map[string]any{}
    var last string
    for rows.Next() {
        var id, typ, st, lastErr, url string
        var attempts int
        var nextAt sql.NullTime
        if err := rows.Scan(&id, &typ, &st, &attempts, &nextAt, &lastErr, &url); err != nil { return nil, "", err }
        m := map[string]any{"id": id, "eventType": typ, "status": st, "attempts": attempts, "url": url}
        if nextAt.Valid { m["nextAttemptAt"] = nextAt.Time }
        if lastErr != "" { m["lastError"] = lastErr }
        out = append(out, m)
        last = id
    }
    next := ""
    if len(out) == limit { next = last }
    return out, next, nil
}

func (p *Postgres) RetryWebhookDelivery(ctx context.Context, tenantID, id string) error {
    _, err := p.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status='pending', next_attempt_at=now() WHERE tenant_id=$1 AND id=$2`, tenantID, id)
    return err
}

func (p *Postgres) RouteStats(ctx context.Context, tenantID, planDate string) (map[string]any, error) {
    row := p.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT r.id) AS routes, COUNT(l.id) AS legs,
        COALESCE(SUM(l.dist_m),0) AS dist_m, COALESCE(SUM(l.drive_sec),0) AS drive_sec,
        COALESCE(SUM(CASE WHEN l.kind='break' THEN 1 ELSE 0 END),0) AS breaks,
        COALESCE(SUM(CASE WHEN l.kind='break' THEN l.break_sec ELSE 0 END),0) AS break_sec
        FROM routes r LEFT JOIN route_legs l ON l.route_id = r.id AND l.tenant_id = r.tenant_id
        WHERE r.tenant_id=$1 AND r.plan_date=$2`, tenantID, planDate)
    var routes, legs int
    var dist, drive int64
    var breaks, breakSec int64
    if err := row.Scan(&routes, &legs, &dist, &drive, &breaks, &breakSec); err != nil { return nil, err }
    avg := 0.0
    if routes > 0 { avg = float64(legs) / float64(routes) }
    return map[string]any{
        "routes": routes,
        "legs": legs,
        "totalDistM": dist,
        "totalDriveSec": drive,
        "avgLegsPerRoute": avg,
        "breaks": breaks,
        "breakSec": breakSec,
    }, nil
}

func (p *Postgres) WebhookMetrics(ctx context.Context, tenantID string, since time.Time, eventType, status string, codeMin, codeMax int, buckets []int) ([]map[string]any, error) {
    // Build dynamic latency buckets from thresholds (sorted)
    // Default thresholds if none provided
    if len(buckets) == 0 { buckets = []int{100, 500, 1000} }
    // Build SELECT parts
    sel := `SELECT event_type, status, COUNT(*) AS cnt, COALESCE(AVG(latency_ms),0) AS avg_latency_ms`
    for i, edge := range buckets {
        if i == 0 {
            sel += fmt.Sprintf(", SUM(CASE WHEN COALESCE(latency_ms,0) < %d THEN 1 ELSE 0 END) AS b%d", edge, i)
        } else {
            prev := buckets[i-1]
            sel += fmt.Sprintf(", SUM(CASE WHEN COALESCE(latency_ms,0) >= %d AND COALESCE(latency_ms,0) < %d THEN 1 ELSE 0 END) AS b%d", prev, edge, i)
        }
    }
    // last overflow bucket
    lastIdx := len(buckets)
    lastEdge := buckets[len(buckets)-1]
    sel += fmt.Sprintf(", SUM(CASE WHEN COALESCE(latency_ms,0) >= %d THEN 1 ELSE 0 END) AS b%d", lastEdge, lastIdx)
    sel += ", SUM(CASE WHEN COALESCE(response_code,0) BETWEEN 200 AND 299 THEN 1 ELSE 0 END) AS c2xx, SUM(CASE WHEN COALESCE(response_code,0) BETWEEN 300 AND 399 THEN 1 ELSE 0 END) AS c3xx, SUM(CASE WHEN COALESCE(response_code,0) BETWEEN 400 AND 499 THEN 1 ELSE 0 END) AS c4xx, SUM(CASE WHEN COALESCE(response_code,0) BETWEEN 500 AND 599 THEN 1 ELSE 0 END) AS c5xx"
    q := sel + ` FROM webhook_deliveries WHERE tenant_id=$1 AND updated_at >= $2`
    args := []any{tenantID, since}
    idx := 3
    if eventType != "" { q += ` AND event_type=$` + fmt.Sprint(idx); args = append(args, eventType); idx++ }
    if status != "" { q += ` AND status=$` + fmt.Sprint(idx); args = append(args, status); idx++ }
    if codeMin > 0 { q += ` AND COALESCE(response_code,0) >= $` + fmt.Sprint(idx); args = append(args, codeMin); idx++ }
    if codeMax > 0 { q += ` AND COALESCE(response_code,0) <= $` + fmt.Sprint(idx); args = append(args, codeMax); idx++ }
    q += ` GROUP BY event_type, status`
    rows, err := p.db.QueryContext(ctx, q, args...)
    if err != nil { return nil, err }
    defer rows.Close()
    out := []map[string]any{}
    for rows.Next() {
        // Prepare scan targets dynamically
        cols := 4 + len(buckets) + 1 + 4 // et, st, cnt, avg + buckets + overflow + code classes
        scan := make([]any, cols)
        var et, st string
        var cnt, avg int64
        scan[0] = &et; scan[1] = &st; scan[2] = &cnt; scan[3] = &avg
        bucketVals := make([]int64, len(buckets)+1)
        for i := range bucketVals { scan[4+i] = &bucketVals[i] }
        base := 4 + len(bucketVals)
        var c2, c3, c4, c5 int64
        scan[base+0] = &c2; scan[base+1] = &c3; scan[base+2] = &c4; scan[base+3] = &c5
        if err := rows.Scan(scan...); err != nil { return nil, err }
        // Legacy object buckets mapping (best-effort) using default labels
        legacy := map[string]int64{}
        if len(buckets) >= 1 { legacy[fmt.Sprintf("lt%d", buckets[0])] = bucketVals[0] }
        if len(buckets) >= 2 { legacy[fmt.Sprintf("p%d_%d", buckets[0], buckets[1])] = bucketVals[1] }
        if len(buckets) >= 3 { legacy[fmt.Sprintf("p%d_%d", buckets[1], buckets[2])] = bucketVals[2] }
        // overflow
        legacy[fmt.Sprintf("gte%d", buckets[len(buckets)-1])] = bucketVals[len(bucketVals)-1]
        // Build response
        out = append(out, map[string]any{
            "eventType": et,
            "status": st,
            "count": cnt,
            "avgLatencyMs": avg,
            "latencyBuckets": legacy,
            "latencyBucketEdges": buckets,
            "latencyBucketCounts": bucketVals,
            "codeClasses": map[string]int64{"c2xx": c2, "c3xx": c3, "c4xx": c4, "c5xx": c5},
        })
    }
    return out, nil
}

func (p *Postgres) FindRoutesByStop(ctx context.Context, tenantID, stopID string) ([]string, error) {
    rows, err := p.db.QueryContext(ctx, `SELECT DISTINCT route_id::text FROM route_legs WHERE tenant_id=$1 AND to_stop_id=$2`, tenantID, stopID)
    if err != nil { return nil, err }
    defer rows.Close()
    var ids []string
    for rows.Next() { var id string; if err := rows.Scan(&id); err != nil { return nil, err }; ids = append(ids, id) }
    return ids, nil
}

func (p *Postgres) SavePlanMetrics(ctx context.Context, tenantID, planDate, algo string, metrics map[string]any) error {
    id := uuid.New().String()
    _, err := p.db.ExecContext(ctx, `INSERT INTO plan_metrics (id, tenant_id, plan_date, algo, iterations, improvements, accepted_worse, best_cost, final_cost, removal_selects, insert_selects, init_temp, cooling, init_removal_weights, init_insertion_weights, objectives, final_removal_weights, final_insertion_weights)
        VALUES ($1,$2,$3,$4,COALESCE($5,0),COALESCE($6,0),COALESCE($7,0),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
        ON CONFLICT (tenant_id, plan_date, algo) DO UPDATE SET
          iterations=COALESCE($5,0), improvements=COALESCE($6,0), accepted_worse=COALESCE($7,0), best_cost=$8, final_cost=$9, removal_selects=$10, insert_selects=$11, init_temp=$12, cooling=$13, init_removal_weights=$14, init_insertion_weights=$15, objectives=$16, final_removal_weights=$17, final_insertion_weights=$18, created_at=now()`,
        id, tenantID, planDate, algo,
        metrics["iterations"], metrics["improvements"], metrics["acceptedWorse"], metrics["bestCost"], metrics["finalCost"], metrics["removalSelects"], metrics["insertSelects"], metrics["initTemp"], metrics["cooling"], metrics["initRemovalWeights"], metrics["initInsertionWeights"], metrics["objectives"], metrics["finalRemovalWeights"], metrics["finalInsertionWeights"],
    )
    return err
}

func (p *Postgres) ListPlanMetrics(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error) {
    base := `SELECT algo, iterations, improvements, accepted_worse, best_cost, final_cost, removal_selects, insert_selects, init_temp, cooling, init_removal_weights, init_insertion_weights, objectives, final_removal_weights, final_insertion_weights FROM plan_metrics WHERE tenant_id=$1 AND plan_date=$2`
    args := []any{tenantID, planDate}
    if algo != "" { base += ` AND algo=$3`; args = append(args, algo) }
    rows, err := p.db.QueryContext(ctx, base, args...)
    if err != nil { return nil, err }
    defer rows.Close()
    out := []map[string]any{}
    for rows.Next() {
        var algo string
        var iter, imp, aw int
        var best, final sql.NullFloat64
        var rem, ins any
        var initTemp, cooling sql.NullFloat64
        var initRem, initIns any
        var objectives any
        var finRem, finIns any
        if err := rows.Scan(&algo, &iter, &imp, &aw, &best, &final, &rem, &ins, &initTemp, &cooling, &initRem, &initIns, &objectives, &finRem, &finIns); err != nil { return nil, err }
        item := map[string]any{
            "algo": algo,
            "iterations": iter,
            "improvements": imp,
            "acceptedWorse": aw,
            "bestCost": best.Float64,
            "finalCost": final.Float64,
            "removalSelects": rem,
            "insertSelects": ins,
            "initTemp": initTemp.Float64,
            "cooling": cooling.Float64,
            "initRemovalWeights": initRem,
            "initInsertionWeights": initIns,
            "objectives": objectives,
            "finalRemovalWeights": finRem,
            "finalInsertionWeights": finIns,
        }
        out = append(out, item)
    }
    return out, nil
}

func (p *Postgres) SavePlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string, snaps []map[string]any) error {
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func(){ _ = tx.Rollback() }()
    for _, s0 := range snaps {
        id := uuid.New().String()
        _, err := tx.ExecContext(ctx, `INSERT INTO plan_metrics_weights (id, tenant_id, plan_date, algo, iteration, removal_weights, insertion_weights)
            VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, tenantID, planDate, algo, s0["iteration"], s0["removal"], s0["insertion"])
        if err != nil { return err }
    }
    return tx.Commit()
}

func (p *Postgres) ListPlanMetricsWeights(ctx context.Context, tenantID, planDate, algo string) ([]map[string]any, error) {
    rows, err := p.db.QueryContext(ctx, `SELECT iteration, removal_weights, insertion_weights FROM plan_metrics_weights WHERE tenant_id=$1 AND plan_date=$2 AND algo=$3 ORDER BY iteration`, tenantID, planDate, algo)
    if err != nil { return nil, err }
    defer rows.Close()
    out := []map[string]any{}
    for rows.Next() {
        var iter int
        var rem, ins any
        if err := rows.Scan(&iter, &rem, &ins); err != nil { return nil, err }
        out = append(out, map[string]any{"iteration": iter, "removal": rem, "insertion": ins})
    }
    return out, nil
}

func (p *Postgres) GetOptimizerConfig(ctx context.Context, tenantID string) (map[string]any, error) {
    row := p.db.QueryRowContext(ctx, `SELECT config FROM optimizer_config WHERE tenant_id=$1`, tenantID)
    var js []byte
    if err := row.Scan(&js); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return nil, nil }
        return nil, err
    }
    var cfg map[string]any
    if err := json.Unmarshal(js, &cfg); err != nil { return nil, err }
    return cfg, nil
}

func (p *Postgres) SaveOptimizerConfig(ctx context.Context, tenantID string, cfg map[string]any) error {
    id := tenantID
    _, err := p.db.ExecContext(ctx, `INSERT INTO optimizer_config (tenant_id, config, updated_at) VALUES ($1, $2, now())
        ON CONFLICT (tenant_id) DO UPDATE SET config=$2, updated_at=now()`, id, cfg)
    return err
}

func (p *Postgres) ListWebhookDLQ(ctx context.Context, tenantID, eventType string, olderThan time.Time, codeMin, codeMax int, errorQuery, cursor string, limit int) ([]map[string]any, string, error) {
    if limit <= 0 || limit > 500 { limit = 100 }
    base := `SELECT id::text, delivery_id::text, event_type, url, last_error, attempts, created_at, COALESCE(response_code,0), COALESCE(latency_ms,0) FROM webhook_dlq WHERE tenant_id=$1`
    args := []any{tenantID}
    idx := 2
    if eventType != "" { base += ` AND event_type=$` + fmt.Sprint(idx); args = append(args, eventType); idx++ }
    if !olderThan.IsZero() { base += ` AND created_at < $` + fmt.Sprint(idx); args = append(args, olderThan); idx++ }
    if codeMin > 0 { base += ` AND COALESCE(response_code,0) >= $` + fmt.Sprint(idx); args = append(args, codeMin); idx++ }
    if codeMax > 0 { base += ` AND COALESCE(response_code,0) <= $` + fmt.Sprint(idx); args = append(args, codeMax); idx++ }
    if errorQuery != "" { base += ` AND last_error ILIKE $` + fmt.Sprint(idx); args = append(args, "%"+errorQuery+"%"); idx++ }
    order := ` ORDER BY id`
    var rows *sql.Rows
    var err error
    if cursor != "" {
        q := base + ` AND id::text > $` + fmt.Sprint(idx) + order + ` LIMIT $` + fmt.Sprint(idx+1)
        args = append(args, cursor, limit)
        rows, err = p.db.QueryContext(ctx, q, args...)
    } else {
        q := base + order + ` LIMIT $` + fmt.Sprint(idx)
        args = append(args, limit)
        rows, err = p.db.QueryContext(ctx, q, args...)
    }
    if err != nil { return nil, "", err }
    defer rows.Close()
    out := []map[string]any{}
    var last string
    for rows.Next() {
        var id, delID, et, url, errStr string
        var attempts int
        var created time.Time
        var code, latency int
        if err := rows.Scan(&id, &delID, &et, &url, &errStr, &attempts, &created, &code, &latency); err != nil { return nil, "", err }
        out = append(out, map[string]any{"id": id, "deliveryId": delID, "eventType": et, "url": url, "lastError": errStr, "attempts": attempts, "createdAt": created, "responseCode": code, "latencyMs": latency})
        last = id
    }
    next := ""
    if len(out) == limit { next = last }
    return out, next, nil
}

func (p *Postgres) RequeueWebhookDLQ(ctx context.Context, tenantID, id string) error {
    // Fetch DLQ entry
    var delID, et, url, secret string
    var payload []byte
    err := p.db.QueryRowContext(ctx, `SELECT COALESCE(delivery_id::text,''), event_type, url, COALESCE(secret,''), payload FROM webhook_dlq WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&delID, &et, &url, &secret, &payload)
    if err != nil { return err }
    // Enqueue new delivery
    _, e2 := p.EnqueueWebhook(ctx, tenantID, delID, et, url, secret, payload)
    if e2 != nil { return e2 }
    // Delete from DLQ
    _, e3 := p.db.ExecContext(ctx, `DELETE FROM webhook_dlq WHERE tenant_id=$1 AND id=$2`, tenantID, id)
    return e3
}

func (p *Postgres) RequeueWebhookDLQBulk(ctx context.Context, tenantID string, ids []string) error {
    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func(){ _ = tx.Rollback() }()
    for _, id := range ids {
        var delID, et, url, secret string
        var payload []byte
        if err := tx.QueryRowContext(ctx, `SELECT COALESCE(delivery_id::text,''), event_type, url, COALESCE(secret,''), payload FROM webhook_dlq WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&delID, &et, &url, &secret, &payload); err != nil { return err }
        // enqueue
        if _, err := p.EnqueueWebhook(ctx, tenantID, delID, et, url, secret, payload); err != nil { return err }
        if _, err := tx.ExecContext(ctx, `DELETE FROM webhook_dlq WHERE tenant_id=$1 AND id=$2`, tenantID, id); err != nil { return err }
    }
    return tx.Commit()
}

func (p *Postgres) DeleteWebhookDLQBulk(ctx context.Context, tenantID string, ids []string, olderThan time.Time) error {
    if len(ids) > 0 {
        // delete listed ids
        for _, id := range ids {
            if _, err := p.db.ExecContext(ctx, `DELETE FROM webhook_dlq WHERE tenant_id=$1 AND id=$2`, tenantID, id); err != nil { return err }
        }
        return nil
    }
    if !olderThan.IsZero() {
        _, err := p.db.ExecContext(ctx, `DELETE FROM webhook_dlq WHERE tenant_id=$1 AND created_at < $2`, tenantID, olderThan)
        return err
    }
    return nil
}

func computeDedupKey(payload []byte) string {
    // try to parse JSON and use id
    var m map[string]any
    if json.Unmarshal(payload, &m) == nil {
        if v, ok := m["id"].(string); ok && v != "" {
            return v
        }
    }
    sum := sha256.Sum256(payload)
    return hex.EncodeToString(sum[:8])
}

func (p *Postgres) ListActiveRoutesForDriver(ctx context.Context, tenantID, driverID string) ([]string, error) {
    rows, err := p.db.QueryContext(ctx, `SELECT id::text FROM routes WHERE tenant_id=$1 AND driver_id=$2 AND COALESCE(status,'') <> 'completed'`, tenantID, driverID)
    if err != nil { return nil, err }
    defer rows.Close()
    ids := []string{}
    for rows.Next() { var id string; if err := rows.Scan(&id); err != nil { return nil, err }; ids = append(ids, id) }
    return ids, nil
}
func (p *Postgres) AdvanceRoute(ctx context.Context, tenantID, routeID string, req model.AdvanceRequest) (model.AdvanceResponse, error) {
    // Policy check using route.auto_advance
    rcur, err := p.GetRoute(ctx, tenantID, routeID)
    if err != nil { return model.AdvanceResponse{}, err }
    // Determine current leg (first non-visited)
    var curLegID, curToStopID sql.NullString
    var curSeq sql.NullInt32
    err = p.db.QueryRowContext(ctx, `SELECT id::text, to_stop_id::text, seq FROM route_legs 
        WHERE tenant_id=$1 AND route_id=$2 AND COALESCE(status,'') <> 'visited' ORDER BY seq LIMIT 1`, tenantID, routeID).Scan(&curLegID, &curToStopID, &curSeq)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur}, nil
        }
        return model.AdvanceResponse{}, err
    }

    alerts := []model.PolicyAlert{}
    if !req.Force && rcur.AutoAdvance != nil {
        pol := rcur.AutoAdvance
        if !pol.Enabled {
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
        }
        reason := req.Reason
        if reason == "arrive" { reason = "geofence_arrive" }
        if reason == "pod" { reason = "pod_ack" }
        if pol.RequirePoD && reason != "pod_ack" {
            alerts = append(alerts, model.PolicyAlert{Reason: "require_pod", TS: time.Now().UTC().Format(time.RFC3339)})
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
        }
        if pol.Trigger != "" && reason != "" && pol.Trigger != reason {
            alerts = append(alerts, model.PolicyAlert{Reason: "trigger_mismatch", TS: time.Now().UTC().Format(time.RFC3339)})
            return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
        }
        // Min dwell time: since last arrive
        if pol.MinDwellSec > 0 {
            var ts time.Time
            // per-stop dwell: check last arrive for this stopId
            err := p.db.QueryRowContext(ctx, `SELECT ts FROM events WHERE tenant_id=$1 AND entity_id=$2 AND type='arrive' AND payload->>'stopId' = $3 ORDER BY ts DESC LIMIT 1`, tenantID, routeID, curToStopID.String).Scan(&ts)
            if err == nil {
                if time.Since(ts) < time.Duration(pol.MinDwellSec)*time.Second {
                    alerts = append(alerts, model.PolicyAlert{Reason: "min_dwell", TS: time.Now().UTC().Format(time.RFC3339)})
                    return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
                }
            }
        }
        // Grace period after trigger event
        if pol.GracePeriodSec > 0 && reason != "" {
            // map reason to event type
            evt := reason
            if evt == "pod_ack" { evt = "pod" }
            if evt == "geofence_arrive" { evt = "arrive" }
            var ts time.Time
            err := p.db.QueryRowContext(ctx, `SELECT ts FROM events WHERE tenant_id=$1 AND entity_id=$2 AND type=$3 AND payload->>'stopId' = $4 ORDER BY ts DESC LIMIT 1`, tenantID, routeID, evt, curToStopID.String).Scan(&ts)
            if err == nil {
                if time.Since(ts) < time.Duration(pol.GracePeriodSec)*time.Second {
                    alerts = append(alerts, model.PolicyAlert{Reason: "grace_period", TS: time.Now().UTC().Format(time.RFC3339)})
                    return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
                }
            }
        }
        // Moving lock: block if last speed > 3 kph
        if pol.MovingLock {
            var sp sql.NullFloat64
            _ = p.db.QueryRowContext(ctx, `SELECT (payload->>'speedKph')::double precision FROM events WHERE tenant_id=$1 AND entity_id=$2 AND type='location' ORDER BY ts DESC LIMIT 1`, tenantID, routeID).Scan(&sp)
            if sp.Valid && sp.Float64 > 3.0 {
                alerts = append(alerts, model.PolicyAlert{Reason: "moving_lock", TS: time.Now().UTC().Format(time.RFC3339)})
                return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
            }
        }
        // HoS: block if exceeded per-policy or driver in break/off
        if pol.HosMaxDriveSec > 0 {
            var sumDrive sql.NullInt64
            _ = p.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(drive_sec),0) FROM route_legs WHERE tenant_id=$1 AND route_id=$2 AND status='visited'`, tenantID, routeID).Scan(&sumDrive)
            if sumDrive.Valid && int(sumDrive.Int64) >= pol.HosMaxDriveSec {
                // emit policy.alert and block
                _ = p.emitEvent(ctx, tenantID, "policy.alert", map[string]any{"routeId": routeID, "reason": "hos.break.required", "ts": time.Now().UTC().Format(time.RFC3339)})
                alerts = append(alerts, model.PolicyAlert{Reason: "hos.break.required", TS: time.Now().UTC().Format(time.RFC3339)})
                return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
            }
        }
        // If driver assigned and hos_state.break true, block
        var driverID sql.NullString
        _ = p.db.QueryRowContext(ctx, `SELECT driver_id::text FROM routes WHERE tenant_id=$1 AND id=$2`, tenantID, routeID).Scan(&driverID)
        if driverID.Valid {
            var js []byte
            _ = p.db.QueryRowContext(ctx, `SELECT hos_state FROM drivers WHERE tenant_id=$1 AND id=$2`, tenantID, driverID.String).Scan(&js)
            if len(js) > 0 {
                var m map[string]any
                if json.Unmarshal(js, &m) == nil {
                    if br, ok := m["break"].(bool); ok && br {
                        _ = p.emitEvent(ctx, tenantID, "policy.alert", map[string]any{"routeId": routeID, "reason": "hos.break.in.progress", "ts": time.Now().UTC().Format(time.RFC3339)})
                        alerts = append(alerts, model.PolicyAlert{Reason: "hos.break.in.progress", TS: time.Now().UTC().Format(time.RFC3339)})
                        return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
                    }
                    if st, ok := m["status"].(string); ok && st == "off" {
                        _ = p.emitEvent(ctx, tenantID, "policy.alert", map[string]any{"routeId": routeID, "reason": "hos.shift.off", "ts": time.Now().UTC().Format(time.RFC3339)})
                        alerts = append(alerts, model.PolicyAlert{Reason: "hos.shift.off", TS: time.Now().UTC().Format(time.RFC3339)})
                        return model.AdvanceResponse{Result: model.AdvanceResult{RouteID: routeID, TS: time.Now().UTC().Format(time.RFC3339), Changed: false}, Route: rcur, Alerts: alerts}, nil
                    }
                }
            }
        }
    }

    tx, err := p.db.BeginTx(ctx, nil)
    if err != nil { return model.AdvanceResponse{}, err }
    defer func(){ _ = tx.Rollback() }()

    // Use identified current leg
    legID := curLegID
    toStopID := curToStopID
    var fromStopID sql.NullString
    err = tx.QueryRowContext(ctx, `SELECT from_stop_id::text FROM route_legs WHERE tenant_id=$1 AND route_id=$2 AND id=$3`, tenantID, routeID, legID.String).Scan(&fromStopID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) { /* continue, fromStopID empty */ } else {
            return model.AdvanceResponse{}, err
        }
    }
    // Mark it visited
    _, err = tx.ExecContext(ctx, `UPDATE route_legs SET status='visited' WHERE tenant_id=$1 AND route_id=$2 AND id=$3`, tenantID, routeID, legID.String)
    if err != nil { return model.AdvanceResponse{}, err }
    // Set next leg in_progress
    _, err = tx.ExecContext(ctx, `UPDATE route_legs SET status='in_progress' WHERE tenant_id=$1 AND route_id=$2 AND seq = (SELECT seq+1 FROM route_legs WHERE tenant_id=$1 AND route_id=$2 AND id=$3)`, tenantID, routeID, legID.String)
    if err != nil { return model.AdvanceResponse{}, err }

    if err := tx.Commit(); err != nil { return model.AdvanceResponse{}, err }
    r, err := p.GetRoute(ctx, tenantID, routeID)
    if err != nil { return model.AdvanceResponse{}, err }
    // Prepare result
    res := model.AdvanceResult{
        RouteID:    routeID,
        FromLegID:  legID.String,
        FromStopID: toStopID.String,
        TS:         time.Now().UTC().Format(time.RFC3339),
        Changed:    true,
    }
    // Compute toLegId/ToStopID from refreshed route (if exists)
    for i := range r.Legs {
        if r.Legs[i].ID == legID.String {
            if i+1 < len(r.Legs) {
                res.ToLegID = r.Legs[i+1].ID
                res.ToStopID = r.Legs[i+1].ToStopID
            }
            break
        }
    }
    return model.AdvanceResponse{Result: res, Route: r, Alerts: alerts}, nil
}

// emitEvent inserts an event and enqueues webhooks for it (best-effort).
func (p *Postgres) emitEvent(ctx context.Context, tenantID, eventType string, data map[string]any) error {
    id := uuid.New().String()
    // insert event record
    _, _ = p.db.ExecContext(ctx, `INSERT INTO events (id, tenant_id, type, ts, payload, source) VALUES ($1,$2,$3,now(),$4,$5)`, id, tenantID, eventType, data, "policy")
    subs, err := p.GetSubscriptionsForEvent(ctx, tenantID, eventType)
    if err != nil { return err }
    if len(subs) == 0 { return nil }
    payload := map[string]any{"id": id, "type": eventType, "tenantId": tenantID, "ts": time.Now().UTC().Format(time.RFC3339), "data": data}
    body, _ := json.Marshal(payload)
    for _, s := range subs { _, _ = p.EnqueueWebhook(ctx, tenantID, s.ID, eventType, s.URL, s.Secret, body) }
    return nil
}

// PlanRoutes creates a simple single route with naive ETAs by chaining pending stops.
func (p *Postgres) PlanRoutes(ctx context.Context, req model.OptimizeRequest) ([]model.Route, string, error) {
    // Fetch candidate stops (pending with coordinates)
    rows, err := p.db.QueryContext(ctx, `SELECT id::text, lat, lng, service_time_sec, lower(time_window) AS tw_start, upper(time_window) AS tw_end FROM stops WHERE tenant_id=$1 AND status='pending' AND lat IS NOT NULL AND lng IS NOT NULL ORDER BY id LIMIT 500`, req.TenantID)
    if err != nil { return nil, "", err }
    type st struct{ id string; lat, lng float64; svc int; twStart, twEnd *time.Time }
    var stops []st
    for rows.Next() {
        var s st
        var tws, twe sql.NullTime
        if err := rows.Scan(&s.id, &s.lat, &s.lng, &s.svc, &tws, &twe); err != nil { rows.Close(); return nil, "", err }
        if tws.Valid { t := tws.Time; s.twStart = &t }
        if twe.Valid { t := twe.Time; s.twEnd = &t }
        stops = append(stops, s)
    }
    rows.Close()
    n := len(stops)
    if n < 2 {
        // Create empty route
        id := uuid.New().String()
        _, err := p.db.ExecContext(ctx, `INSERT INTO routes (id, tenant_id, version, plan_date, status) VALUES ($1,$2,$3,$4,$5)`, id, req.TenantID, 1, req.PlanDate, "planned")
        if err != nil { return nil, "", err }
        r, err := p.GetRoute(ctx, req.TenantID, id)
        if err != nil { return nil, "", err }
        return []model.Route{r}, fmt.Sprintf("opt_%d", time.Now().UnixNano()), nil
    }
    // Determine number of routes
    k := len(req.VehiclePool)
    if k <= 0 {
        k = int(math.Min(3, math.Ceil(float64(n)/20.0)))
        if k <= 0 { k = 1 }
    }
    // Select k seeds (farthest-first)
    seeds := []int{0}
    for len(seeds) < k && len(seeds) < n {
        maxd := -1.0
        maxi := -1
        for i := 0; i < n; i++ {
            skip := false
            for _, sidx := range seeds { if sidx == i { skip = true; break } }
            if skip { continue }
            // distance to nearest seed
            mind := math.MaxFloat64
            for _, sidx := range seeds {
                d := haversineMeters(stops[i].lat, stops[i].lng, stops[sidx].lat, stops[sidx].lng)
                if d < mind { mind = d }
            }
            if mind > maxd { maxd = mind; maxi = i }
        }
        if maxi >= 0 { seeds = append(seeds, maxi) } else { break }
    }
    // Assign stops to nearest seed
    clusters := make([][]int, len(seeds))
    for i := 0; i < n; i++ {
        // find nearest seed
        best := 0
        bestd := math.MaxFloat64
        for si, sidx := range seeds {
            d := haversineMeters(stops[i].lat, stops[i].lng, stops[sidx].lat, stops[sidx].lng)
            if d < bestd { bestd = d; best = si }
        }
        clusters[best] = append(clusters[best], i)
    }
    // Load depots (geofences of type 'hub')
    depRows, err := p.db.QueryContext(ctx, `SELECT id::text, lat, lng FROM geofences WHERE tenant_id=$1 AND type='hub' AND lat IS NOT NULL AND lng IS NOT NULL`, req.TenantID)
    if err != nil { return nil, "", err }
    type depot struct{ id string; lat, lng float64 }
    var depots []depot
    for depRows.Next() {
        var d depot
        if err := depRows.Scan(&d.id, &d.lat, &d.lng); err != nil { depRows.Close(); return nil, "", err }
        depots = append(depots, d)
    }
    depRows.Close()

    // ALNS strategy branch
    if strings.ToLower(req.Algorithm) == "alns" {
        // HoS planning parameters
        hosMax := 0
        breakSec := 1800
        if req.Constraints != nil {
            if v, ok := req.Constraints["hosMaxDriveSec"]; ok {
                switch x := v.(type) { case float64: hosMax = int(x); case int: hosMax = x }
            }
            if v, ok := req.Constraints["breakSec"]; ok {
                switch x := v.(type) { case float64: breakSec = int(x); case int: breakSec = x }
            }
        }
        // Objectives: overlay request over defaults
        obj := map[string]float64{"driveTime": 1, "lateness": 4, "failed": 50, "distance": 0.1}
        if req.Objectives != nil { for k, v := range req.Objectives { obj[k] = v } }
        prob := opt.Problem{Nodes: make([]opt.Node, len(stops)), SpeedKph: 50, Objectives: obj, HosMaxDriveSec: hosMax, BreakSec: breakSec,
            InitialTemp: req.InitTemp, Cooling: req.Cooling, InitialRemovalWeights: req.RemovalWeights, InitialInsertionWeights: req.InsertionWeights}
        // Vehicles: from pool if provided else derived count
        var vehicles []opt.Vehicle
        if len(req.VehiclePool) > 0 {
            for _, vid := range req.VehiclePool {
                var cw, cv sql.NullFloat64
                var skillsStr sql.NullString
                _ = p.db.QueryRowContext(ctx, `SELECT (capacity->>'weight')::double precision, (capacity->>'volume')::double precision, array_to_string(skills, ',') FROM vehicles WHERE tenant_id=$1 AND id=$2`, req.TenantID, vid).Scan(&cw, &cv, &skillsStr)
                veh := opt.Vehicle{ID: vid, CapWeight: cw.Float64, CapVolume: cv.Float64}
                if skillsStr.Valid && skillsStr.String != "" { veh.Skills = strings.Split(skillsStr.String, ",") }
                vehicles = append(vehicles, veh)
            }
        } else {
            k := int(math.Min(3, math.Ceil(float64(n)/20.0)))
            if k <= 0 { k = 1 }
            for i := 0; i < k; i++ { vehicles = append(vehicles, opt.Vehicle{ID: uuid.New().String(), CapWeight: 0, CapVolume: 0}) }
        }
        if len(depots) > 0 {
            d := depots[0]
            for i := range vehicles { vehicles[i].StartLatLng = &[2]float64{d.lat, d.lng}; vehicles[i].EndLatLng = &[2]float64{d.lat, d.lng} }
        }
        prob.Vehicles = vehicles
        base := time.Unix(0,0)
        for i := range stops {
            var tw *opt.TW
            if stops[i].twStart != nil || stops[i].twEnd != nil {
                tw = &opt.TW{}
                if stops[i].twStart != nil { tw.Start = base.Add(stops[i].twStart.Sub(base)) }
                if stops[i].twEnd != nil { tw.End = base.Add(stops[i].twEnd.Sub(base)) }
            }
            // required skills
            var skillsStr sql.NullString
            _ = p.db.QueryRowContext(ctx, `SELECT array_to_string(required_skills, ',') FROM stops WHERE tenant_id=$1 AND id=$2`, req.TenantID, stops[i].id).Scan(&skillsStr)
            var skills []string
            if skillsStr.Valid && skillsStr.String != "" { skills = strings.Split(skillsStr.String, ",") }
            // Demand weights/volumes not yet modeled in stops query; set to zero for now
            prob.Nodes[i] = opt.Node{ID: stops[i].id, Lat: stops[i].lat, Lng: stops[i].lng, ServiceSec: stops[i].svc, TW: tw, Demand: opt.Demand{Weight: 0, Volume: 0}, Skills: skills}
        }
        // time budget and iterations config
        tb := 300 * time.Millisecond
        if req.TimeBudgetMs > 0 { tb = time.Duration(req.TimeBudgetMs) * time.Millisecond }
        if req.MaxIterations > 0 { prob.IterationsLimit = req.MaxIterations }
        sol, pm := opt.Solve(prob, 0, tb)
        // record planner metrics (DB + in-memory)
        _ = p.SavePlanMetrics(ctx, req.TenantID, req.PlanDate, "alns", map[string]any{
            "iterations": pm.Iterations,
            "improvements": pm.Improvements,
            "acceptedWorse": pm.AcceptedWorse,
            "bestCost": pm.BestCost,
            "finalCost": pm.FinalCost,
            "removalSelects": []int{pm.RemovalSelects[0], pm.RemovalSelects[1]},
            "insertSelects": []int{pm.InsertSelects[0], pm.InsertSelects[1]},
            "initTemp": prob.InitialTemp,
            "cooling": prob.Cooling,
            "initRemovalWeights": prob.InitialRemovalWeights,
            "initInsertionWeights": prob.InitialInsertionWeights,
            "finalRemovalWeights": []float64{pm.FinalRemovalWeights[0], pm.FinalRemovalWeights[1]},
            "finalInsertionWeights": []float64{pm.FinalInsertionWeights[0], pm.FinalInsertionWeights[1]},
            "objectives": obj,
        })
        opt.RecordMetrics(req.TenantID, req.PlanDate, "alns", pm)
        // persist weight snapshots
        if len(pm.Snapshots) > 0 {
            snaps := make([]map[string]any, 0, len(pm.Snapshots))
            for _, s0 := range pm.Snapshots {
                snaps = append(snaps, map[string]any{
                    "iteration": s0.Iteration,
                    "removal": []float64{s0.Removal[0], s0.Removal[1]},
                    "insertion": []float64{s0.Insertion[0], s0.Insertion[1]},
                })
            }
            _ = p.SavePlanMetricsWeights(ctx, req.TenantID, req.PlanDate, "alns", snaps)
        }
        results := []model.Route{}
        for _, plan := range sol.Plans {
            if len(plan.Order) == 0 { continue }
            rid := uuid.New().String()
            if _, err := p.db.ExecContext(ctx, `INSERT INTO routes (id, tenant_id, version, plan_date, status) VALUES ($1,$2,$3,$4,$5)`, rid, req.TenantID, 1, req.PlanDate, "planned"); err != nil { return nil, "", err }
            curr := time.Now().UTC()
            seq := 1
            driveCum := 0
            if len(depots) > 0 {
                first := stops[plan.Order[0]]
                d := depots[0]
                dist := int(math.Round(haversineMeters(d.lat, d.lng, first.lat, first.lng)))
                drive := int(math.Round(float64(dist) / (50_000.0/3600.0)))
                etaA := curr.Add(time.Duration(drive) * time.Second)
                if first.twStart != nil && etaA.Before(*first.twStart) { etaA = *first.twStart }
                etaD := etaA.Add(time.Duration(first.svc) * time.Second)
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,'in_progress')`, uuid.New().String(), req.TenantID, rid, seq, nil, first.id, dist, drive, etaA, etaD); err != nil { return nil, "", err }
                curr = etaD
                driveCum += drive
                seq++
            }
            for i := 0; i < len(plan.Order)-1; i++ {
                a := stops[plan.Order[i]]
                b := stops[plan.Order[i+1]]
                dist := int(math.Round(haversineMeters(a.lat, a.lng, b.lat, b.lng)))
                drive := int(math.Round(float64(dist) / (50_000.0/3600.0)))
            if hosMax > 0 && driveCum+drive > hosMax && seq > 1 {
                // plan a break leg
                etaA0 := curr
                etaD0 := curr.Add(time.Duration(breakSec) * time.Second)
                if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'break',$5,NULL,NULL,0,0,$6,$7,'pending')`, uuid.New().String(), req.TenantID, rid, seq, breakSec, etaA0, etaD0); err != nil { return nil, "", err }
                _ = p.emitEvent(ctx, req.TenantID, "hos.break.planned", map[string]any{"routeId": rid, "breakSec": breakSec, "start": etaA0.Format(time.RFC3339), "end": etaD0.Format(time.RFC3339)})
                curr = etaD0
                driveCum = 0
                seq++
            }
                etaA := curr.Add(time.Duration(drive) * time.Second)
                if b.twStart != nil && etaA.Before(*b.twStart) { etaA = *b.twStart }
                etaD := etaA.Add(time.Duration(b.svc) * time.Second)
                status := "pending"
                if seq == 1 && len(depots) == 0 { status = "in_progress" }
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,$11)`, uuid.New().String(), req.TenantID, rid, seq, a.id, b.id, dist, drive, etaA, etaD, status); err != nil { return nil, "", err }
                curr = etaD
                driveCum += drive
                seq++
            }
            if len(depots) > 0 {
                last := stops[plan.Order[len(plan.Order)-1]]
                d := depots[0]
                dist := int(math.Round(haversineMeters(last.lat, last.lng, d.lat, d.lng)))
                drive := int(math.Round(float64(dist) / (50_000.0/3600.0)))
                etaA := curr.Add(time.Duration(drive) * time.Second)
                etaD := etaA
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,'pending')`, uuid.New().String(), req.TenantID, rid, seq, last.id, nil, dist, drive, etaA, etaD); err != nil { return nil, "", err }
            }
            r, _ := p.GetRoute(ctx, req.TenantID, rid)
            results = append(results, r)
        }
        return results, fmt.Sprintf("opt_%d", time.Now().UnixNano()), nil
    }
    // HoS planning parameters for greedy planner
    hosMax := 0
    breakSec := 1800
    if req.Constraints != nil {
        if v, ok := req.Constraints["hosMaxDriveSec"]; ok { switch x := v.(type) { case float64: hosMax = int(x); case int: hosMax = x } }
        if v, ok := req.Constraints["breakSec"]; ok { switch x := v.(type) { case float64: breakSec = int(x); case int: breakSec = x } }
    }
    // Build routes per cluster
    results := []model.Route{}
    for ci, idxs := range clusters {
        if len(idxs) < 2 {
            // single-stop cluster, create empty route record
            rid := uuid.New().String()
            if _, err := p.db.ExecContext(ctx, `INSERT INTO routes (id, tenant_id, version, plan_date, status) VALUES ($1,$2,$3,$4,$5)`, rid, req.TenantID, 1, req.PlanDate, "planned"); err != nil { return nil, "", err }
            r, _ := p.GetRoute(ctx, req.TenantID, rid)
            results = append(results, r)
            continue
        }
        // order by nearest neighbor starting at seed
        startIdx := seeds[ci]
        used := make(map[int]bool)
        order := []int{startIdx}
        used[startIdx] = true
        for len(order) < len(idxs) {
            last := order[len(order)-1]
            best := -1
            bestd := math.MaxFloat64
            for _, j := range idxs {
                if used[j] { continue }
                d := haversineMeters(stops[last].lat, stops[last].lng, stops[j].lat, stops[j].lng)
                if d < bestd { bestd = d; best = j }
            }
            if best >= 0 { order = append(order, best); used[best] = true } else { break }
        }
        // Improve ordering via 2-opt
        nodes := make([]opt.StopNode, len(stops))
        for i := range stops { nodes[i] = opt.StopNode{Lat: stops[i].lat, Lng: stops[i].lng} }
        order = opt.ImproveOrder2Opt(nodes, order, 2)

        rid := uuid.New().String()
        if _, err := p.db.ExecContext(ctx, `INSERT INTO routes (id, tenant_id, version, plan_date, status) VALUES ($1,$2,$3,$4,$5)`, rid, req.TenantID, 1, req.PlanDate, "planned"); err != nil { return nil, "", err }
        curr := time.Now().UTC()
        seq := 1
        driveCum := 0
        // optional depot start/end
        var startLat, startLng float64
        var startStopID string
        if len(depots) > 0 {
            // choose nearest depot to first stop
            first := stops[order[0]]
            best := 0
            bestd := math.MaxFloat64
            for i, d := range depots {
                dd := haversineMeters(first.lat, first.lng, d.lat, d.lng)
                if dd < bestd { bestd = dd; best = i }
            }
            startLat, startLng = depots[best].lat, depots[best].lng
            startStopID = depots[best].id // not a stop id, but we keep relation via geofence id in from_stop_id as empty
            // Create a depot->first leg with empty from_stop_id
            aLat, aLng := startLat, startLng
            b := stops[order[0]]
            dist := int(math.Round(haversineMeters(aLat, aLng, b.lat, b.lng)))
            drive := int(math.Round(float64(dist) / (50_000.0/3600.0)))
            etaA := curr.Add(time.Duration(drive) * time.Second)
            if b.twStart != nil && etaA.Before(*b.twStart) { etaA = *b.twStart }
            etaD := etaA.Add(time.Duration(b.svc) * time.Second)
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,'in_progress')`,
                uuid.New().String(), req.TenantID, rid, seq, nil, b.id, dist, drive, etaA, etaD); err != nil { return nil, "", err }
            curr = etaD
            driveCum += drive
            seq++
        }
        // intra-stop legs
        for i := 0; i < len(order)-1; i++ {
            a := stops[order[i]]
            b := stops[order[i+1]]
            dist := int(math.Round(haversineMeters(a.lat, a.lng, b.lat, b.lng)))
            drive := int(math.Round(float64(dist) / (50_000.0/3600.0))) // 50 kph
            if hosMax > 0 && driveCum+drive > hosMax && seq > 1 {
                // plan a break leg
                etaA0 := curr
                etaD0 := curr.Add(time.Duration(breakSec) * time.Second)
                if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'break',$5,NULL,NULL,0,0,$6,$7,'pending')`, uuid.New().String(), req.TenantID, rid, seq, breakSec, etaA0, etaD0); err != nil { return nil, "", err }
                _ = p.emitEvent(ctx, req.TenantID, "hos.break.planned", map[string]any{"routeId": rid, "breakSec": breakSec, "start": etaA0.Format(time.RFC3339), "end": etaD0.Format(time.RFC3339)})
                curr = etaD0
                driveCum = 0
                seq++
            }
            etaA := curr.Add(time.Duration(drive) * time.Second)
            // wait if before time window start
            if b.twStart != nil && etaA.Before(*b.twStart) { etaA = *b.twStart }
            etaD := etaA.Add(time.Duration(b.svc) * time.Second)
            status := "pending"
            if seq == 1 && startStopID == "" { status = "in_progress" }
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,$11)`,
                uuid.New().String(), req.TenantID, rid, seq, a.id, b.id, dist, drive, etaA, etaD, status); err != nil { return nil, "", err }
            curr = etaD
            driveCum += drive
            seq++
        }
        // last leg back to depot if present
        if len(depots) > 0 {
            last := stops[order[len(order)-1]]
            dist := int(math.Round(haversineMeters(last.lat, last.lng, startLat, startLng)))
            drive := int(math.Round(float64(dist) / (50_000.0/3600.0)))
            etaA := curr.Add(time.Duration(drive) * time.Second)
            etaD := etaA // no service time at depot
            if _, err := p.db.ExecContext(ctx, `INSERT INTO route_legs (id, tenant_id, route_id, seq, kind, break_sec, from_stop_id, to_stop_id, dist_m, drive_sec, eta_arrival, eta_departure, status) VALUES ($1,$2,$3,$4,'drive',NULL,$5,$6,$7,$8,$9,$10,'pending')`,
                uuid.New().String(), req.TenantID, rid, seq, last.id, nil, dist, drive, etaA, etaD); err != nil { return nil, "", err }
        }
        r, _ := p.GetRoute(ctx, req.TenantID, rid)
        results = append(results, r)
    }
    return results, fmt.Sprintf("opt_%d", time.Now().UnixNano()), nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
    const R = 6371000.0
    dLat := (lat2 - lat1) * math.Pi / 180
    dLon := (lon2 - lon1) * math.Pi / 180
    a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    return R * c
}

func ternary[T any](cond bool, a, b T) T { if cond { return a }; return b }

// HOS
func (p *Postgres) UpdateHOS(ctx context.Context, tenantID, driverID string, upd model.HOSUpdate) (string, map[string]any, error) {
    // naive JSONB update: read-modify-write
    hos := map[string]any{}
    row := p.db.QueryRowContext(ctx, `SELECT hos_state FROM drivers WHERE tenant_id=$1 AND id=$2`, tenantID, driverID)
    var js []byte
    if err := row.Scan(&js); err != nil && !errors.Is(err, sql.ErrNoRows) {
        return "", nil, err
    }
    if len(js) > 0 { _ = json.Unmarshal(js, &hos) }
    status := fmt.Sprint(hos["status"])
    switch upd.Action {
    case "shift_start": status = "on"; hos["shiftStart"] = upd.TS
    case "shift_end": status = "off"; hos["shiftEnd"] = upd.TS
    case "break_start": hos["break"] = true; hos["breakType"] = upd.Type; hos["breakStart"] = upd.TS
    case "break_end": hos["break"] = false; hos["breakEnd"] = upd.TS
    }
    hos["status"] = status
    if upd.Note != "" { hos["note"] = upd.Note }
    _, err := p.db.ExecContext(ctx, `UPDATE drivers SET hos_state=$1 WHERE tenant_id=$2 AND id=$3`, hos, tenantID, driverID)
    if err != nil { return "", nil, err }
    return status, hos, nil
}

// Geofences
func (p *Postgres) CreateGeofence(ctx context.Context, tenantID string, in model.GeofenceInput) (model.Geofence, error) {
    id := uuid.New().String()
    var lat, lng any
    if in.Center != nil { lat, lng = in.Center.Lat, in.Center.Lng }
    _, err := p.db.ExecContext(ctx, `INSERT INTO geofences (id, tenant_id, name, radius_m, type, rules, lat, lng) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, tenantID, in.Name, in.RadiusM, in.Type, in.Rules, lat, lng)
    if err != nil { return model.Geofence{}, err }
    return p.GetGeofence(ctx, tenantID, id)
}

func (p *Postgres) ListGeofences(ctx context.Context, tenantID, cursor string, limit int) ([]model.Geofence, string, error) {
    if limit <= 0 || limit > 500 { limit = 100 }
    var rows *sql.Rows
    var err error
    if cursor != "" {
        rows, err = p.db.QueryContext(ctx, `SELECT id::text, name, radius_m, type, rules, lat, lng FROM geofences WHERE tenant_id=$1 AND id::text > $2 ORDER BY id LIMIT $3`, tenantID, cursor, limit)
    } else {
        rows, err = p.db.QueryContext(ctx, `SELECT id::text, name, radius_m, type, rules, lat, lng FROM geofences WHERE tenant_id=$1 ORDER BY id LIMIT $2`, tenantID, limit)
    }
    if err != nil { return nil, "", err }
    defer rows.Close()
    out := []model.Geofence{}
    var last string
    for rows.Next() {
        var gf model.Geofence
        var lat, lng sql.NullFloat64
        if err := rows.Scan(&gf.ID, &gf.Name, &gf.RadiusM, &gf.Type, &gf.Rules, &lat, &lng); err != nil { return nil, "", err }
        gf.TenantID = tenantID
        if lat.Valid && lng.Valid { gf.Center = &model.GeoPoint{Lat: lat.Float64, Lng: lng.Float64} }
        out = append(out, gf)
        last = gf.ID
    }
    next := ""
    if len(out) == limit { next = last }
    return out, next, nil
}

func (p *Postgres) GetGeofence(ctx context.Context, tenantID, id string) (model.Geofence, error) {
    var gf model.Geofence
    var lat, lng sql.NullFloat64
    row := p.db.QueryRowContext(ctx, `SELECT id::text, name, radius_m, type, rules, lat, lng FROM geofences WHERE tenant_id=$1 AND id=$2`, tenantID, id)
    if err := row.Scan(&gf.ID, &gf.Name, &gf.RadiusM, &gf.Type, &gf.Rules, &lat, &lng); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return gf, ErrNotFound }
        return gf, err
    }
    gf.TenantID = tenantID
    if lat.Valid && lng.Valid { gf.Center = &model.GeoPoint{Lat: lat.Float64, Lng: lng.Float64} }
    return gf, nil
}

func (p *Postgres) PatchGeofence(ctx context.Context, tenantID, id string, in model.GeofenceInput) (model.Geofence, error) {
    // Simple approach: read current, overwrite provided fields, then update
    gf, err := p.GetGeofence(ctx, tenantID, id)
    if err != nil { return gf, err }
    if in.Name != "" { gf.Name = in.Name }
    if in.Type != "" { gf.Type = in.Type }
    if in.RadiusM != 0 { gf.RadiusM = in.RadiusM }
    if in.Rules != nil { gf.Rules = in.Rules }
    var lat, lng any
    if in.Center != nil { lat, lng = in.Center.Lat, in.Center.Lng } else if gf.Center != nil { lat, lng = gf.Center.Lat, gf.Center.Lng }
    _, err = p.db.ExecContext(ctx, `UPDATE geofences SET name=$1, radius_m=$2, type=$3, rules=$4, lat=$5, lng=$6 WHERE tenant_id=$7 AND id=$8`, gf.Name, gf.RadiusM, gf.Type, gf.Rules, lat, lng, tenantID, id)
    if err != nil { return gf, err }
    return p.GetGeofence(ctx, tenantID, id)
}

func (p *Postgres) DeleteGeofence(ctx context.Context, tenantID, id string) error {
    _, err := p.db.ExecContext(ctx, `DELETE FROM geofences WHERE tenant_id=$1 AND id=$2`, tenantID, id)
    return err
}
// (old minimal PlanRoutes removed)

// Helpers
func nullIfEmpty(s string) any { if s == "" { return nil }; return s }
func toJSON(m map[string]any) any { if m == nil { return nil }; return m }
func mediaURL(m *model.PoDMedia) any { if m == nil { return nil }; return m.UploadURL }
func mediaHash(m *model.PoDMedia) any { if m == nil { return nil }; return m.SHA256 }

// naive pq string[] encoder using JSONB cast in SQL could be preferred; here we just pass nil or []string via driver
func pqStringArray(v []string) any {
    if len(v) == 0 { return nil }
    return v
}
