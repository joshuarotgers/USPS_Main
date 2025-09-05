package metrics

import (
    "sync"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/collectors"
)

var (
    // Registry is the dedicated Prometheus registry for the API
    Registry = prometheus.NewRegistry()
    // HTTPRequests counts requests by method, path, and status
    HTTPRequests = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests."},
        []string{"method", "path", "status"},
    )
    // HTTPDuration records request durations in seconds
    HTTPDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds.", Buckets: prometheus.DefBuckets},
        []string{"method", "path", "status"},
    )

    // WebhookDeliveries counts webhook delivery outcomes by event type and status
    WebhookDeliveries = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "webhook_deliveries_total", Help: "Webhook deliveries by event type and status."},
        []string{"event_type", "status"},
    )
    // WebhookLatency tracks webhook delivery latencies in milliseconds
    WebhookLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "webhook_delivery_latency_ms", Help: "Webhook delivery latency in ms.", Buckets: []float64{10, 50, 100, 200, 500, 1000, 2000, 5000}},
        []string{"event_type", "status"},
    )
)

// RegisterDefault registers collectors to the default registry.
func RegisterDefault() {
    regOnce.Do(func(){
        Registry.MustRegister(HTTPRequests)
        Registry.MustRegister(HTTPDuration)
        Registry.MustRegister(WebhookDeliveries)
        Registry.MustRegister(WebhookLatency)
        // Go/process collectors on our registry
        Registry.MustRegister(collectors.NewGoCollector())
        Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
    })
}

var regOnce sync.Once
