package main

import (
    "fmt"
    "flag"
    "log"
    "log/slog"
    "net"
    "net/http"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
    "golang.org/x/time/rate"

    "gpsnav/internal/api"
    "gpsnav/internal/metrics"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    migrate := flag.Bool("migrate", true, "run DB migrations on startup (Postgres mode)")
    flag.Parse()
    if !*migrate { os.Setenv("DB_MIGRATE", "false") }
    srvDeps, err := api.NewServer()
    if err != nil {
        log.Fatalf("failed to init server: %v", err)
    }

    mux := http.NewServeMux()

    // Orders
    mux.HandleFunc("/v1/orders", srvDeps.OrdersHandler)

    // Optimization
    mux.HandleFunc("/v1/optimize", srvDeps.OptimizeHandler)
    mux.HandleFunc("/v1/optimizer/config", srvDeps.OptimizerConfigHandler)
    mux.HandleFunc("/v1/admin/optimizer/config", srvDeps.AdminOptimizerConfigHandler)

    // Routes
    mux.HandleFunc("/v1/routes", srvDeps.RoutesIndexHandler)
    mux.HandleFunc("/v1/routes/", srvDeps.RouteByIDHandler) // includes /assign, /advance, /events/stream
    mux.HandleFunc("/v1/eta/stream", srvDeps.ETAStreamHandler)
    
    // Driver events, PoD, subscriptions
    mux.HandleFunc("/v1/driver-events", srvDeps.DriverEventsHandler)
    mux.HandleFunc("/v1/pod", srvDeps.PoDHandler)
    mux.HandleFunc("/v1/subscriptions", srvDeps.SubscriptionsHandler)
    mux.HandleFunc("/v1/subscriptions/", srvDeps.SubscriptionByIDHandler)

    // Drivers
    mux.HandleFunc("/v1/drivers/", srvDeps.DriversHandler)

    // Geofences
    mux.HandleFunc("/v1/geofences", srvDeps.GeofencesHandler)
    mux.HandleFunc("/v1/geofences/", srvDeps.GeofenceByIDHandler)

    // Media
    mux.HandleFunc("/v1/media/presign", srvDeps.MediaPresignHandler)

    // Health
    mux.HandleFunc("/healthz", srvDeps.HealthHandler)
    mux.HandleFunc("/readyz", srvDeps.ReadyHandler)
    // Docs / OpenAPI
    mux.HandleFunc("/openapi.yaml", srvDeps.OpenAPIHandler)
    mux.HandleFunc("/docs", srvDeps.DocsHandler)
    // Metrics
    metrics.RegisterDefault()
    mux.Handle("/metrics", promhttp.Handler())

    // Admin
    mux.HandleFunc("/v1/admin/webhook-deliveries", srvDeps.WebhookDeliveriesHandler)
    mux.HandleFunc("/v1/admin/webhook-deliveries/", srvDeps.WebhookDeliveryRetryHandler)
    mux.HandleFunc("/v1/admin/routes/stats", srvDeps.RouteStatsHandler)
    mux.HandleFunc("/v1/admin/plan-metrics", srvDeps.PlanMetricsHandler)
    mux.HandleFunc("/v1/admin/plan-metrics/weights", srvDeps.PlanMetricsWeightsHandler)
    mux.HandleFunc("/v1/admin/webhook-metrics", srvDeps.WebhookMetricsHandler)
    mux.HandleFunc("/v1/admin/webhook-dlq", srvDeps.WebhookDLQHandler)
    mux.HandleFunc("/v1/admin/webhook-dlq/", srvDeps.WebhookDLQHandler)

    // GraphQL subscription bridge (SSE) for route events
    mux.HandleFunc("/graphql/subscriptions/route-events", func(w http.ResponseWriter, r *http.Request) {
        // bridge to existing SSE handler: /v1/routes/{routeId}/events/stream
        id := r.URL.Query().Get("routeId")
        if id == "" { http.Error(w, "routeId required", http.StatusBadRequest); return }
        // rewrite path and delegate
        r.URL.Path = "/v1/routes/" + id + "/events/stream"
        srvDeps.RouteByIDHandler(w, r)
    })

    // GraphQL WebSocket subscriptions endpoint
    mux.HandleFunc("/graphql/ws", srvDeps.GraphQLWSHandler)
    // Minimal GraphQL HTTP endpoint (queries)
    mux.HandleFunc("/graphql", srvDeps.GraphQLHTTPHandler)

    addr := ":8080"
    if v := os.Getenv("PORT"); v != "" {
        addr = ":" + v
    }

    handler := logMiddleware(requestIDMiddleware(corsMiddleware(rateLimitMiddleware(mux))))
    srv := &http.Server{
        Addr:              addr,
        Handler:           handler,
        ReadHeaderTimeout: 5 * time.Second,
    }

    log.Printf("API listening on %s", addr)
    // Start webhook worker
    if srvDeps.Pub != nil {
        worker := srvDeps.NewWebhookWorker()
        worker.Start()
    }
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }
}

type statusWriter struct {
    http.ResponseWriter
    status int
}
func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func logMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        sw := &statusWriter{ResponseWriter: w, status: 200}
        next.ServeHTTP(sw, r)
        dur := time.Since(start)
        rid := r.Header.Get("X-Request-Id")
        logger := slog.Default()
        logger.Info("http_request",
            slog.String("method", r.Method),
            slog.String("path", r.URL.Path),
            slog.String("client", clientIP(r)),
            slog.Int("status", sw.status),
            slog.String("dur", dur.String()),
            slog.String("request_id", rid),
        )
        // record metrics
        st := fmt.Sprintf("%d", sw.status)
        metrics.HTTPRequests.WithLabelValues(r.Method, r.URL.Path, st).Inc()
        metrics.HTTPDuration.WithLabelValues(r.Method, r.URL.Path, st).Observe(dur.Seconds())
    })
}

// Helper to satisfy reference and avoid unused imports in stubs
var _ = fmt.Sprintf

// ---- Request ID middleware ----
func requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        rid := r.Header.Get("X-Request-Id")
        if rid == "" { rid = uuid.New().String() }
        w.Header().Set("X-Request-Id", rid)
        // propagate on request for downstream
        r.Header.Set("X-Request-Id", rid)
        next.ServeHTTP(w, r)
    })
}

// ---- CORS middleware ----
func corsMiddleware(next http.Handler) http.Handler {
    allowed := strings.Split(strings.TrimSpace(os.Getenv("ALLOW_ORIGINS")), ",")
    allowAll := false
    m := map[string]struct{}{}
    for _, o := range allowed {
        o = strings.TrimSpace(o)
        if o == "" { continue }
        if o == "*" { allowAll = true }
        m[o] = struct{}{}
    }
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        if allowAll && origin != "" { w.Header().Set("Access-Control-Allow-Origin", origin) }
        if !allowAll {
            if _, ok := m[origin]; ok && origin != "" {
                w.Header().Set("Access-Control-Allow-Origin", origin)
            }
        }
        w.Header().Set("Vary", "Origin")
        if r.Method == http.MethodOptions {
            w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-Id,X-Tenant-Id,X-Role,X-Driver-Id")
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// ---- Rate limit middleware ----
type rlStore struct {
    mu sync.Mutex
    m  map[string]*rate.Limiter
    r  rate.Limit
    b  int
}

func newRLStore(rps rate.Limit, burst int) *rlStore { return &rlStore{m: map[string]*rate.Limiter{}, r: rps, b: burst} }
func (s *rlStore) limiter(key string) *rate.Limiter {
    s.mu.Lock(); defer s.mu.Unlock()
    if lm := s.m[key]; lm != nil { return lm }
    lm := rate.NewLimiter(s.r, s.b)
    s.m[key] = lm
    return lm
}

func rateLimitMiddleware(next http.Handler) http.Handler {
    rps := 20.0
    burst := 40
    if v := os.Getenv("RATE_RPS"); v != "" { if x, err := strconv.ParseFloat(v, 64); err == nil && x > 0 { rps = x } }
    if v := os.Getenv("RATE_BURST"); v != "" { if x, err := strconv.Atoi(v); err == nil && x > 0 { burst = x } }
    store := newRLStore(rate.Limit(rps), burst)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := clientIP(r)
        if !store.limiter(ip).Allow() {
            http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func clientIP(r *http.Request) string {
    if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
        parts := strings.Split(xf, ",")
        return strings.TrimSpace(parts[0])
    }
    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err == nil { return host }
    return r.RemoteAddr
}
