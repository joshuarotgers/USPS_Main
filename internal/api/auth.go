// Package api implements HTTP handlers and helpers for the GPSNav service.
package api

import (
    "net/http"
    "strings"
)

type Principal struct {
	Tenant   string
	Role     string // admin, dispatcher, driver, customer
	DriverID string
}

// getPrincipal extracts tenant and role from JWT or headers.
// - If Authorization: Bearer is present, uses configured verifier (dev/hmac/jwks).
// - Else falls back to headers for dev.
func (s *Server) getPrincipal(r *http.Request) Principal {
    authz := r.Header.Get("Authorization")
    if strings.HasPrefix(strings.ToLower(authz), "bearer ") && s.Auth != nil {
        tok := strings.TrimSpace(authz[len("Bearer "):])
        if pr, err := s.Auth.Verify(tok); err == nil {
            // Normalize tenant for underlying store (e.g., map aliases to UUID)
            t := s.normalizeTenantID(pr.Tenant)
            return Principal{Tenant: t, Role: pr.Role, DriverID: pr.DriverID}
        }
    }
    tenant := r.Header.Get("X-Tenant-Id")
    role := r.Header.Get("X-Role")
    driverID := r.Header.Get("X-Driver-Id")
    if tenant == "" {
        tenant = "t_demo"
    }
    tenant = s.normalizeTenantID(tenant)
    if role == "" {
        role = "admin"
    }
    return Principal{Tenant: tenant, Role: role, DriverID: driverID}
}

// IsAdmin reports whether the principal has the admin role.
func (p Principal) IsAdmin() bool { return p.Role == "admin" }
