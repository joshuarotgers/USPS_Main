package api

import (
    "bytes"
    "encoding/json"
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func newTestServer(t *testing.T) *Server {
    t.Helper()
    s, err := NewServer()
    if err != nil { t.Fatalf("NewServer: %v", err) }
    return s
}

func TestHealthReady(t *testing.T) {
    s := newTestServer(t)
    rr := httptest.NewRecorder()
    s.HealthHandler(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
    if rr.Code != 200 { t.Fatalf("health: got %d", rr.Code) }
    rr = httptest.NewRecorder()
    s.ReadyHandler(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
    if rr.Code != 200 { t.Fatalf("ready: got %d", rr.Code) }
}

func TestOrdersCreateList(t *testing.T) {
    s := newTestServer(t)
    // POST /v1/orders
    body := []byte(`{"tenantId":"t_test","orders":[{"externalRef":"O1","stops":[{"type":"pickup","location":{"lat":1,"lng":2}},{"type":"dropoff","location":{"lat":1.1,"lng":2.1}}]}]}`)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    s.OrdersHandler(rr, req)
    if rr.Code != http.StatusAccepted { t.Fatalf("orders create: got %d", rr.Code) }
    // GET /v1/orders
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodGet, "/v1/orders?limit=5", nil)
    s.OrdersHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("orders list: got %d", rr.Code) }
}

func TestOptimizeAndRoute(t *testing.T) {
    s := newTestServer(t)
    // Optimize
    oreq := map[string]any{"tenantId":"t_test","planDate":"2024-01-01","algorithm":"greedy"}
    b,_ := json.Marshal(oreq)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/optimize", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    s.OptimizeHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("optimize: %d", rr.Code) }
}

func TestRoutesIndexAndGraphQL(t *testing.T) {
    s := newTestServer(t)
    // seed a route
    oreq := map[string]any{"tenantId":"t_test","planDate":"2024-01-01","algorithm":"greedy"}
    b,_ := json.Marshal(oreq)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/optimize", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    s.OptimizeHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("optimize: %d", rr.Code) }

    // list routes
    rr = httptest.NewRecorder()
    s.RoutesIndexHandler(rr, httptest.NewRequest(http.MethodGet, "/v1/routes", nil))
    if rr.Code != 200 { t.Fatalf("routes index: %d", rr.Code) }

    // GraphQL: routes
    var body = []byte(`{"query":"query { routes }"}`)
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    s.GraphQLHTTPHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("graphql routes: %d", rr.Code) }

    // GraphQL: route(id)
    // decode first route id via REST index
    rr2 := httptest.NewRecorder()
    s.RoutesIndexHandler(rr2, httptest.NewRequest(http.MethodGet, "/v1/routes", nil))
    var idx struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
    if err := json.Unmarshal(rr2.Body.Bytes(), &idx); err == nil && len(idx.Items) > 0 {
        rid := idx.Items[0].ID
        qb, _ := json.Marshal(map[string]any{
            "query":     "query($id: ID!) { route(id: $id) }",
            "variables": map[string]any{"id": rid},
        })
        rr = httptest.NewRecorder()
        req = httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(qb))
        req.Header.Set("Content-Type", "application/json")
        s.GraphQLHTTPHandler(rr, req)
        if rr.Code != 200 { t.Fatalf("graphql route: %d", rr.Code) }
    }
}

func TestDriverRoutesList(t *testing.T) {
    s := newTestServer(t)
    // create a route then assign to driver drv1
    oreq := map[string]any{"tenantId":"t_test","planDate":"2024-01-01","algorithm":"greedy"}
    b,_ := json.Marshal(oreq)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/optimize", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.OptimizeHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("optimize: %d", rr.Code) }
    var ores struct{ Routes []struct{ ID string `json:"id"` } `json:"routes"` }
    _ = json.Unmarshal(rr.Body.Bytes(), &ores)
    rid := ores.Routes[0].ID
    // assign
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/v1/routes/"+rid+"/assign", bytes.NewReader([]byte(`{"driverId":"drv1","vehicleId":"veh1"}`)))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.RouteByIDHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("assign: %d", rr.Code) }
    // list driver routes
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodGet, "/v1/drivers/drv1/routes", nil)
    req.Header.Set("X-Tenant-Id", "t_test")
    s.DriversHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("list driver routes: %d", rr.Code) }
}

func TestPoDHandler(t *testing.T) {
    s := newTestServer(t)
    body := []byte(`{"tenantId":"t_test","orderId":"ord_1","stopId":"s1","type":"signature"}`)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/pod", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    s.PoDHandler(rr, req)
    if rr.Code != http.StatusCreated { t.Fatalf("pod: %d", rr.Code) }
}

func TestAssignAdvanceEnqueuesWebhook(t *testing.T) {
    s := newTestServer(t)
    // Create subscription for stop.advanced
    subBody := []byte(`{"tenantId":"t_test","url":"https://example.invalid/webhook","events":["stop.advanced"],"secret":"shh"}`)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions", bytes.NewReader(subBody))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.SubscriptionsHandler(rr, req)
    if rr.Code != http.StatusCreated { t.Fatalf("create sub: %d", rr.Code) }

    // Optimize to get a route
    oreq := map[string]any{"tenantId":"t_test","planDate":"2024-01-01","algorithm":"greedy"}
    ob,_ := json.Marshal(oreq)
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/v1/optimize", bytes.NewReader(ob))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Role", "admin")
    s.OptimizeHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("optimize: %d", rr.Code) }
    var ores struct{ Routes []struct{ ID string `json:"id"` } `json:"routes"` }
    if err := json.Unmarshal(rr.Body.Bytes(), &ores); err != nil { t.Fatalf("decode optimize: %v", err) }
    if len(ores.Routes) == 0 { t.Fatalf("no routes returned") }
    rid := ores.Routes[0].ID

    // Assign route
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/v1/routes/"+rid+"/assign", bytes.NewReader([]byte(`{"driverId":"drv1","vehicleId":"veh1"}`)))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.RouteByIDHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("assign: %d", rr.Code) }

    // Advance route (should emit webhook delivery)
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/v1/routes/"+rid+"/advance", bytes.NewReader([]byte(`{}`)))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.RouteByIDHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("advance: %d", rr.Code) }

    // List admin deliveries; expect at least one stop.advanced item
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodGet, "/v1/admin/webhook-deliveries?limit=5", nil)
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.WebhookDeliveriesHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("deliveries: %d", rr.Code) }
    var dres struct{ Items []map[string]any `json:"items"` }
    if err := json.Unmarshal(rr.Body.Bytes(), &dres); err != nil { t.Fatalf("decode deliveries: %v", err) }
    if len(dres.Items) == 0 { t.Fatalf("expected at least one delivery") }
    // optional check type
    if et, ok := dres.Items[0]["eventType"].(string); ok && et == "" {
        t.Fatalf("eventType should not be empty")
    }
}

// sseRecorder is a minimal ResponseWriter that implements http.Flusher
// and captures writes for SSE tests.
type sseRecorder struct {
    hdr  http.Header
    buf  bytes.Buffer
    code int
}

func (r *sseRecorder) Header() http.Header { if r.hdr == nil { r.hdr = http.Header{} }; return r.hdr }
func (r *sseRecorder) WriteHeader(c int) { r.code = c }
func (r *sseRecorder) Write(p []byte) (int, error) { return r.buf.Write(p) }
func (r *sseRecorder) Flush() {}

func TestRouteEventsSSE(t *testing.T) {
    s := newTestServer(t)
    // Create a route via optimize
    oreq := map[string]any{"tenantId":"t_test","planDate":"2024-01-01","algorithm":"greedy"}
    ob,_ := json.Marshal(oreq)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/v1/optimize", bytes.NewReader(ob))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Tenant-Id", "t_test")
    req.Header.Set("X-Role", "admin")
    s.OptimizeHandler(rr, req)
    if rr.Code != 200 { t.Fatalf("optimize: %d", rr.Code) }
    var ores struct{ Routes []struct{ ID string `json:"id"` } `json:"routes"` }
    if err := json.Unmarshal(rr.Body.Bytes(), &ores); err != nil { t.Fatalf("decode optimize: %v", err) }
    if len(ores.Routes) == 0 { t.Fatalf("no routes returned") }
    rid := ores.Routes[0].ID

    // Prepare SSE request with cancelable context
    sseReq := httptest.NewRequest(http.MethodGet, "/v1/routes/"+rid+"/events/stream", nil)
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    sseReq = sseReq.WithContext(ctx)
    sseReq.Header.Set("X-Tenant-Id", "t_test")
    sseReq.Header.Set("X-Role", "admin")

    rec := &sseRecorder{}
    done := make(chan struct{})
    go func() {
        s.RouteByIDHandler(rec, sseReq)
        close(done)
    }()

    // Give handler time to subscribe and send heartbeat
    time.Sleep(50 * time.Millisecond)
    // Publish an event
    s.Broker.Publish(rid, SSEEvent{Type: "policy.alert", Data: map[string]any{"routeId": rid}})

    // Wait up to 500ms for the event to appear in buffer
    deadline := time.Now().Add(500 * time.Millisecond)
    for time.Now().Before(deadline) {
        if bytes.Contains(rec.buf.Bytes(), []byte("event: policy.alert")) {
            break
        }
        time.Sleep(10 * time.Millisecond)
    }
    if !bytes.Contains(rec.buf.Bytes(), []byte("event: policy.alert")) {
        t.Fatalf("SSE did not contain expected event. Body: %s", rec.buf.String())
    }
    // Ensure handler exits on context cancel
    cancel()
    select {
    case <-done:
    case <-time.After(200 * time.Millisecond):
        t.Fatal("handler did not exit after cancel")
    }
}
