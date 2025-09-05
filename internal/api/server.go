package api

import (
    "context"
    "net/http"
    "os"
    "strings"

    "gpsnav/internal/store"
    "gpsnav/internal/webhooks"
    "gpsnav/internal/auth"
)

type Server struct {
    Store store.Store
    Pub   *webhooks.Publisher
    Auth  *auth.Verifier
    Broker EventBroker
}

// NewServer creates a Server. If DATABASE_URL is unset, uses in-memory store.
func NewServer() (*Server, error) {
    dsn := os.Getenv("DATABASE_URL")
    var s store.Store
    if strings.TrimSpace(dsn) == "" {
        s = store.NewMemory()
    } else {
        sp, err := store.NewPostgres(dsn)
        if err != nil {
            return nil, err
        }
        // Run migrations (dev helper)
        if os.Getenv("DB_MIGRATE") != "false" {
            _ = sp.MigrateDir("db/migrations")
        }
        s = sp
    }
    // Broker selection
    var broker EventBroker
    if os.Getenv("REDIS_URL") != "" {
        if rb, err := NewRedisBroker(); err == nil { broker = rb } else { broker = NewBroker() }
    } else {
        broker = NewBroker()
    }
    return &Server{Store: s, Pub: webhooks.NewPublisher(s), Auth: auth.NewVerifierFromEnv(), Broker: broker}, nil
}

func (s *Server) withTenant(r *http.Request) (context.Context, string) {
    // For now, get tenant from header; in production decode from JWT.
    tenant := r.Header.Get("X-Tenant-Id")
    if tenant == "" { tenant = "t_demo" }
    ctx := context.WithValue(r.Context(), ctxKeyTenant{}, tenant)
    return ctx, tenant
}

type ctxKeyTenant struct{}

// NewWebhookWorker creates a background worker for webhook deliveries.
func (s *Server) NewWebhookWorker() *webhooks.Worker {
    return webhooks.NewWorker(s.Store)
}
