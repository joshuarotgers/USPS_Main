package api

import (
    "net/http"
    "strings"

    iauth "gpsnav/internal/auth"
)

type Principal struct {
    Tenant string
    Role   string // admin, dispatcher, driver, customer
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
            return Principal{Tenant: pr.Tenant, Role: pr.Role, DriverID: pr.DriverID}
        }
    }
    tenant := r.Header.Get("X-Tenant-Id")
    role := r.Header.Get("X-Role")
    driverID := r.Header.Get("X-Driver-Id")
    if tenant == "" { tenant = "t_demo" }
    if role == "" { role = "admin" }
    return Principal{Tenant: tenant, Role: role, DriverID: driverID}
}

func (p Principal) IsAdmin() bool { return p.Role == "admin" }

// helper to adapt auth.Principal to local type when needed
func fromAuthPrincipal(p iauth.Principal) Principal { return Principal{Tenant: p.Tenant, Role: p.Role, DriverID: p.DriverID} }
