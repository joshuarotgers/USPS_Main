package main

import (
    "fmt"
    "flag"
    "log"
    "net/http"
    "os"
    "time"

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

    srv := &http.Server{
        Addr:              addr,
        Handler:           logMiddleware(mux),
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
        sw, ok := w.(*statusWriter)
        if !ok { sw = &statusWriter{ResponseWriter: w, status: 200} } else { sw.status = 200 }
        next.ServeHTTP(sw, r)
        dur := time.Since(start)
        log.Printf("%s %s %s %v", r.RemoteAddr, r.Method, r.URL.Path, dur)
        // record metrics
        st := fmt.Sprintf("%d", sw.status)
        metrics.HTTPRequests.WithLabelValues(r.Method, r.URL.Path, st).Inc()
        metrics.HTTPDuration.WithLabelValues(r.Method, r.URL.Path, st).Observe(dur.Seconds())
    })
}

// Helper to satisfy reference and avoid unused imports in stubs
var _ = fmt.Sprintf
