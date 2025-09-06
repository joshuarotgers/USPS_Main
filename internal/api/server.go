package api

import (
    "context"
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/google/uuid"
    "gpsnav/internal/auth"
    "gpsnav/internal/store"
    "gpsnav/internal/webhooks"
)

// Server bundles API dependencies and HTTP handlers.
type Server struct {
	Store  store.Store
	Pub    *webhooks.Publisher
	Auth   *auth.Verifier
	Broker EventBroker
	Locs   *LocationCache
    // Demo tenant UUID used when running with Postgres so that dev aliases like
    // "t_demo" can be mapped to a valid FK-safe UUID.
    DemoTenantID string
}

// NewServer creates a Server. If DATABASE_URL is unset, uses in-memory store.
func NewServer() (*Server, error) {
	dsn := os.Getenv("DATABASE_URL")
	var s store.Store
    var demoTenantID string
    if strings.TrimSpace(dsn) == "" {
        s = store.NewMemory()
    } else {
        sp, err := store.NewPostgres(dsn)
        if err != nil {
            return nil, err
        }
        // Run migrations (dev helper)
        if os.Getenv("DB_MIGRATE") != "false" {
            log.Printf("DB_MIGRATE enabled: applying migrations from db/migrations")
            if err := sp.MigrateDir("db/migrations"); err != nil {
                log.Printf("DB migration error: %v", err)
                return nil, err
            }
            log.Printf("DB migrations applied")
        }
        // Seed demo tenant for dev convenience
        if id, err := sp.EnsureTenantByName(context.Background(), "t_demo"); err == nil {
            demoTenantID = id
        } else {
            log.Printf("Ensure demo tenant failed: %v", err)
        }
        s = sp
    }
	// Broker selection
	var broker EventBroker
	if os.Getenv("REDIS_URL") != "" {
		if rb, err := NewRedisBroker(); err == nil {
			broker = rb
		} else {
			broker = NewBroker()
		}
	} else {
		broker = NewBroker()
	}
	return &Server{Store: s, Pub: webhooks.NewPublisher(s), Auth: auth.NewVerifierFromEnv(), Broker: broker, Locs: NewLocationCache(), DemoTenantID: demoTenantID}, nil
}

func (s *Server) withTenant(r *http.Request) (context.Context, string) {
	// For now, get tenant from header or JWT and normalize for store
	raw := r.Header.Get("X-Tenant-Id")
	if raw == "" {
		raw = "t_demo"
	}
	tenant := s.normalizeTenantID(raw)
	ctx := context.WithValue(r.Context(), ctxKeyTenant{}, tenant)
	return ctx, tenant
}

type ctxKeyTenant struct{}

// NewWebhookWorker creates a background worker for webhook deliveries.
func (s *Server) NewWebhookWorker() *webhooks.Worker {
	return webhooks.NewWorker(s.Store)
}

// normalizeTenantID maps non-UUID tenant aliases (like "t_demo") to a UUID
// when using Postgres. Memory store leaves them unchanged.
func (s *Server) normalizeTenantID(raw string) string {
    // If Postgres is in use, prefer DemoTenantID as the default mapping
    if _, ok := s.Store.(*store.Postgres); ok {
        if _, err := uuid.Parse(strings.TrimSpace(raw)); err == nil {
            return raw
        }
        if s.DemoTenantID != "" {
            return s.DemoTenantID
        }
    }
    if strings.TrimSpace(raw) == "" {
        return "t_demo"
    }
    return raw
}

// normalizeDriverID converts a human-friendly driver key to a stable UUID for
// Postgres-backed stores. For memory store, returns raw unchanged.
func (s *Server) normalizeDriverID(tenantID, raw string) string {
    if _, ok := s.Store.(*store.Postgres); ok {
        if _, err := uuid.Parse(strings.TrimSpace(raw)); err == nil {
            return raw
        }
        // Deterministic UUID v5 using tenant scope to avoid cross-tenant collision
        ns := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tenantID))
        id := uuid.NewSHA1(ns, []byte(strings.TrimSpace(raw)))
        return id.String()
    }
    return raw
}
