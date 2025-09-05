package api

import (
    "encoding/json"
    "net/http"
    "strings"
)

// Minimal GraphQL-like HTTP handler for demo purposes.
// Supports queries:
// - routes: list routes for tenant
// - route(id: $id): get route by id
// Variables may contain {"id":"..."}
func (s *Server) GraphQLHTTPHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { w.WriteHeader(405); return }
    var body struct {
        Query     string                 `json:"query"`
        Variables map[string]any         `json:"variables"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
    q := strings.ToLower(body.Query)
    _, tenant := s.withTenant(r)
    switch {
    case strings.Contains(q, "route("):
        id := ""
        if body.Variables != nil { if v, ok := body.Variables["id"].(string); ok { id = v } }
        if id == "" { writeProblem(w, 400, "Missing id", "", r.URL.Path); return }
        rt, err := s.Store.GetRoute(r.Context(), tenant, id)
        if err != nil { writeProblem(w, 404, "Not Found", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]any{"data": map[string]any{"route": rt}})
    case strings.Contains(q, "routes"):
        cursor := ""; limit := 100
        items, next, err := s.Store.ListRoutes(r.Context(), tenant, cursor, limit)
        if err != nil { writeProblem(w, 500, "List routes failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]any{"data": map[string]any{"routes": items, "nextCursor": next}})
    default:
        writeProblem(w, 400, "Unsupported query", "", r.URL.Path)
    }
}

