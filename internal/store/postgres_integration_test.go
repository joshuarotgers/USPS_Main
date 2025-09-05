//go:build postgres_integration

package store

import (
    "os"
    "testing"
)

func TestPostgresConnectivityAndMigrate(t *testing.T) {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" { t.Skip("DATABASE_URL not set; skipping integration test") }
    p, err := NewPostgres(dsn)
    if err != nil { t.Fatalf("NewPostgres: %v", err) }
    if err := p.Ping(t.Context()); err != nil { t.Fatalf("Ping: %v", err) }
    if err := p.MigrateDir("../../db/migrations"); err != nil { t.Fatalf("MigrateDir: %v", err) }
    // Try simple call
    if _, _, err := p.ListRoutes(t.Context(), "t_demo", "", 1); err != nil { t.Fatalf("ListRoutes: %v", err) }
}

