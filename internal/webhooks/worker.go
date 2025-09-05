package webhooks

import (
    "bytes"
    "context"
    "net/http"
    "time"

    "gpsnav/internal/store"
    "os"
    "strconv"
)

type Worker struct {
    Store store.Store
    HTTP  *http.Client
    Stop  chan struct{}
    MaxAttempts int
}

func NewWorker(s store.Store) *Worker {
    max := 10
    if v := os.Getenv("WEBHOOK_MAX_ATTEMPTS"); v != "" { if n,err := strconv.Atoi(v); err == nil && n>0 { max = n } }
    return &Worker{Store: s, HTTP: &http.Client{Timeout: 5 * time.Second}, Stop: make(chan struct{}), MaxAttempts: max}
}

func (w *Worker) Start() {
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-w.Stop:
                return
            case <-ticker.C:
                w.processOnce()
            }
        }
    }()
}

func (w *Worker) processOnce() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    items, err := w.Store.FetchDueWebhookDeliveries(ctx, 50)
    if err != nil || len(items) == 0 { return }
    for _, it := range items {
        success := false
        next := time.Now().Add(nextBackoff(it.Attempts))
        req, _ := http.NewRequestWithContext(ctx, http.MethodPost, it.URL, bytes.NewReader(it.Payload))
        req.Header.Set("Content-Type", "application/json")
        if it.Secret != "" {
            sig := SignHMAC(it.Secret, it.Payload)
            req.Header.Set("X-Signature", sig)
            req.Header.Set("X-Event-Type", it.EventType)
        }
        start := time.Now()
        resp, err := w.HTTP.Do(req)
        latency := int(time.Since(start).Milliseconds())
        code := 0
        if err == nil && resp != nil {
            code = resp.StatusCode
            if resp.Body != nil { _ = resp.Body.Close() }
            if code >= 200 && code < 300 { success = true }
        }
        lastErr := ""
        if !success && err != nil { lastErr = err.Error() }
        if !success && it.Attempts+1 >= w.MaxAttempts {
            _ = w.Store.FailWebhookDelivery(ctx, it.ID, lastErr, code, latency)
            continue
        }
        _ = w.Store.MarkWebhookDelivery(ctx, it.ID, success, &next, lastErr, code, latency)
    }
}

func nextBackoff(attempts int) time.Duration {
    if attempts < 0 { attempts = 0 }
    if attempts > 10 { attempts = 10 }
    base := time.Second * time.Duration(1<<attempts)
    if base > time.Hour { base = time.Hour }
    return base
}
