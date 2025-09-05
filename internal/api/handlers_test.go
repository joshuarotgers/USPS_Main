package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
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

