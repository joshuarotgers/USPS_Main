package api

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "gpsnav/internal/model"
    "gpsnav/internal/opt"
)

// OrdersHandler handles POST/GET /v1/orders
func (s *Server) OrdersHandler(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        var req struct {
            TenantID string          `json:"tenantId"`
            Orders   []model.OrderIn `json:"orders"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
            return
        }
        if req.TenantID == "" { _, req.TenantID = s.withTenant(r) }
        imp, created, skipped, err := s.Store.CreateOrders(r.Context(), req.TenantID, req.Orders)
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "Create orders failed", err.Error(), r.URL.Path)
            return
        }
        writeJSON(w, http.StatusAccepted, map[string]any{"importId": imp, "created": created, "skipped": skipped})
    case http.MethodGet:
        _, tenant := s.withTenant(r)
        status := r.URL.Query().Get("status")
        cursor := r.URL.Query().Get("cursor")
        limit := 100
        if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
        items, next, err := s.Store.ListOrders(r.Context(), tenant, status, cursor, limit)
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "List orders failed", err.Error(), r.URL.Path)
            return
        }
        writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

// OptimizeHandler handles POST /v1/optimize
func (s *Server) OptimizeHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    p := s.getPrincipal(r)
    if !(p.IsAdmin() || p.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
    var req model.OptimizeRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
        return
    }
    if err := validateOptimizeRequest(&req); err != nil {
        writeProblem(w, http.StatusBadRequest, "Invalid optimize request", err.Error(), r.URL.Path)
        return
    }

    if req.TenantID == "" { _, req.TenantID = s.withTenant(r) }
    routes, batchID, err := s.Store.PlanRoutes(r.Context(), req)
    if err != nil {
        writeProblem(w, http.StatusInternalServerError, "Plan routes failed", err.Error(), r.URL.Path)
        return
    }
    // Publish SSE for planned breaks (webhooks are enqueued by store)
    for _, rt := range routes {
        for _, lg := range rt.Legs {
            if strings.ToLower(lg.Kind) == "break" && lg.BreakSec > 0 {
                evt := SSEEvent{Type: "hos.break.planned", Data: map[string]any{"routeId": rt.ID, "breakSec": lg.BreakSec, "etaStart": lg.ETAArrival, "etaEnd": lg.ETADeparture}}
                s.Broker.Publish(rt.ID, evt)
            }
        }
    }
    writeJSON(w, http.StatusOK, map[string]any{"batchId": batchID, "routes": routes})
}

// OptimizerConfigHandler returns default optimizer configuration
func (s *Server) OptimizerConfigHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/optimizer/config" || r.Method != http.MethodGet { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    defaults := map[string]any{
        "algorithm": "alns",
        "timeBudgetMs": 300,
        "maxIterations": 0,
        "initTemp": 1.0,
        "cooling": 0.995,
        "removalWeights": []float64{1.0, 1.0},
        "insertionWeights": []float64{1.0, 1.0},
        "objectives": map[string]float64{"driveTime": 1, "lateness": 4, "failed": 50, "distance": 0.1},
        "latencyBuckets": []int{100, 500, 1000},
    }
    // overlay tenant config if present
    p := s.getPrincipal(r)
    cfg, _ := s.Store.GetOptimizerConfig(r.Context(), p.Tenant)
    if cfg != nil {
        // merge cfg into defaults
        for k, v := range cfg { defaults[k] = v }
    }
    writeJSON(w, 200, map[string]any{"defaults": defaults})
}

// Admin get/set optimizer tenant config
func (s *Server) AdminOptimizerConfigHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/optimizer/config" { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    switch r.Method {
    case http.MethodGet:
        cfg, _ := s.Store.GetOptimizerConfig(r.Context(), p.Tenant)
        if cfg == nil { cfg = map[string]any{} }
        writeJSON(w, 200, map[string]any{"config": cfg})
    case http.MethodPut:
        var body struct{ Config map[string]any `json:"config"` }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
        if body.Config == nil { writeProblem(w, 400, "Missing config", "", r.URL.Path); return }
        if err := s.Store.SaveOptimizerConfig(r.Context(), p.Tenant, body.Config); err != nil { writeProblem(w, 500, "Save failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]bool{"ok": true})
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

// RouteByIDHandler handles GET/PATCH /v1/routes/{id} and POST /v1/routes/{id}/assign
func (s *Server) RouteByIDHandler(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Path
    // Expected: /v1/routes/{id} or /v1/routes/{id}/assign
    rest := strings.TrimPrefix(path, "/v1/routes/")
    if rest == path || rest == "" {
        writeProblem(w, http.StatusNotFound, "Not Found", "missing id", path)
        return
    }
    parts := strings.Split(rest, "/")
    id := parts[0]
    if len(parts) > 1 && parts[1] == "events" && len(parts) > 2 && parts[2] == "stream" {
        // SSE for route events
        if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
        // RBAC: admin/dispatcher or assigned driver
        pr := s.getPrincipal(r)
        if !(pr.IsAdmin() || pr.Role == "dispatcher") {
            // allow drivers only for their assigned routes
            _, tenant := s.withTenant(r)
            rt, err := s.Store.GetRoute(r.Context(), tenant, id)
            if err != nil { writeProblem(w, 404, "Route not found", err.Error(), r.URL.Path); return }
            if pr.Role != "driver" || pr.DriverID == "" || rt.DriverID == "" || pr.DriverID != rt.DriverID {
                writeProblem(w, 403, "Forbidden", "not authorized for route events", r.URL.Path)
                return
            }
        }
        flusher, ok := w.(http.Flusher)
        if !ok { writeProblem(w, 500, "Streaming unsupported", "", r.URL.Path); return }
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        // subscribe
        ch := s.Broker.Subscribe(id)
        defer s.Broker.Unsubscribe(id, ch)
        // initial heartbeat
        fmt.Fprintf(w, "event: heartbeat\n")
        fmt.Fprintf(w, "data: {\"routeId\":\"%s\",\"ts\":\"%s\"}\n\n", id, time.Now().Format(time.RFC3339))
        flusher.Flush()
        // stream loop
        notify := r.Context().Done()
        for {
            select {
            case <-notify:
                return
            case evt := <-ch:
                b, _ := json.Marshal(evt.Data)
                fmt.Fprintf(w, "event: %s\n", evt.Type)
                fmt.Fprintf(w, "data: %s\n\n", string(b))
                flusher.Flush()
            case <-time.After(15 * time.Second):
                fmt.Fprintf(w, "event: heartbeat\n")
                fmt.Fprintf(w, "data: {\"routeId\":\"%s\",\"ts\":\"%s\"}\n\n", id, time.Now().Format(time.RFC3339))
                flusher.Flush()
            }
        }
    }
    if len(parts) > 1 && parts[1] == "assign" {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        var req model.AssignmentRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
            return
        }
        _, tenant := s.withTenant(r)
        pr := s.getPrincipal(r)
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        route, err := s.Store.AssignRoute(r.Context(), tenant, id, req.DriverID, req.VehicleID, time.Now())
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "Assign route failed", err.Error(), r.URL.Path)
            return
        }
        writeJSON(w, http.StatusOK, route)
        return
    }
    if len(parts) > 1 && parts[1] == "advance" {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        var req model.AdvanceRequest
        // body is optional
        if r.Body != nil {
            _ = json.NewDecoder(r.Body).Decode(&req)
        }
        _, tenant := s.withTenant(r)
        resp, err := s.Store.AdvanceRoute(r.Context(), tenant, id, req)
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "Advance route failed", err.Error(), r.URL.Path)
            return
        }
        if resp.Result.Changed {
            s.Pub.Emit(r.Context(), tenant, "stop.advanced", map[string]any{
                "routeId": resp.Result.RouteID,
                "fromStopId": resp.Result.FromStopID,
                "toStopId": resp.Result.ToStopID,
                "ts": resp.Result.TS,
            })
            s.Broker.Publish(id, SSEEvent{Type: "stop.advanced", Data: map[string]any{
                "routeId": resp.Result.RouteID,
                "fromStopId": resp.Result.FromStopID,
                "toStopId": resp.Result.ToStopID,
                "ts": resp.Result.TS,
            }})
        }
        // Publish policy alerts via SSE
        if len(resp.Alerts) > 0 {
            for _, a := range resp.Alerts {
                s.Broker.Publish(id, SSEEvent{Type: "policy.alert", Data: map[string]any{"routeId": id, "reason": a.Reason, "ts": a.TS}})
            }
        }
        writeJSON(w, http.StatusOK, resp)
        return
    }

    switch r.Method {
    case http.MethodGet:
        _, tenant := s.withTenant(r)
        route, err := s.Store.GetRoute(r.Context(), tenant, id)
        if err != nil {
            writeProblem(w, http.StatusNotFound, "Route not found", err.Error(), r.URL.Path)
            return
        }
        // Optional filtering of break legs
        if inc := r.URL.Query().Get("includeBreaks"); strings.EqualFold(inc, "false") || inc == "0" {
            legs := make([]model.Leg, 0, len(route.Legs))
            for _, l := range route.Legs {
                if strings.ToLower(l.Kind) == "break" { continue }
                legs = append(legs, l)
            }
            route.Legs = legs
        }
        writeJSON(w, http.StatusOK, route)
    case http.MethodPatch:
        // Simulate If-Match handling
        _ = r.Header.Get("If-Match")
        body, _ := io.ReadAll(r.Body)
        _ = body
        _, tenant := s.withTenant(r)
        route, err := s.Store.PatchRoute(r.Context(), tenant, id, model.RoutePatch{Status: "updated"})
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "Update route failed", err.Error(), r.URL.Path)
            return
        }
        writeJSON(w, http.StatusOK, route)
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

// RoutesIndexHandler exists for completeness (not used yet)
func (s *Server) RoutesIndexHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/routes" { writeProblem(w, http.StatusNotFound, "Not Found", "", r.URL.Path); return }
    if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
    _, tenant := s.withTenant(r)
    cursor := r.URL.Query().Get("cursor")
    limit := 100
    if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
    items, next, err := s.Store.ListRoutes(r.Context(), tenant, cursor, limit)
    if err != nil { writeProblem(w, 500, "List routes failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

// DriverEventsHandler handles POST /v1/driver-events
func (s *Server) DriverEventsHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    pr := s.getPrincipal(r)
    if !(pr.IsAdmin() || pr.Role == "driver" || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "driver, dispatcher or admin required", r.URL.Path); return }
    var req struct {
        TenantID string              `json:"tenantId"`
        Events   []model.DriverEvent `json:"events"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
        return
    }
    if req.TenantID == "" { _, req.TenantID = s.withTenant(r) }
    n, err := s.Store.InsertDriverEvents(r.Context(), req.TenantID, req.Events)
    if err != nil {
        writeProblem(w, http.StatusInternalServerError, "Insert events failed", err.Error(), r.URL.Path)
        return
    }
    // Optional: attempt auto-advance if events include triggers (simplified heuristic)
    var adv *model.AdvanceResult
    for _, e := range req.Events {
        if e.Type == "pod" || e.Type == "depart" || e.Type == "arrive" {
            reason := e.Type
            if reason == "pod" { reason = "pod_ack" }
            if reason == "arrive" { reason = "geofence_arrive" }
            res, err := s.Store.AdvanceRoute(r.Context(), req.TenantID, e.RouteID, model.AdvanceRequest{Reason: reason})
            if err == nil && res.Result.Changed {
                tmp := res.Result
                adv = &tmp
                s.Pub.Emit(r.Context(), req.TenantID, "stop.advanced", map[string]any{
                    "routeId": res.Result.RouteID,
                    "fromStopId": res.Result.FromStopID,
                    "toStopId": res.Result.ToStopID,
                    "ts": res.Result.TS,
                })
                break
            }
        }
    }
    resp := map[string]any{"accepted": n, "rejected": 0}
    if adv != nil { resp["advanced"] = []model.AdvanceResult{*adv} }
    writeJSON(w, http.StatusAccepted, resp)
}

// PoDHandler handles POST /v1/pod
func (s *Server) PoDHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    var req model.PoDRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
        return
    }
    id, status, err := s.Store.CreatePoD(r.Context(), req)
    if err != nil {
        writeProblem(w, http.StatusInternalServerError, "Create PoD failed", err.Error(), r.URL.Path)
        return
    }
    // SSE for pod captured to any routes containing this stop
    routes, _ := s.Store.FindRoutesByStop(r.Context(), req.TenantID, req.StopID)
    data := map[string]any{"orderId": req.OrderID, "stopId": req.StopID, "podId": id, "ts": time.Now().UTC().Format(time.RFC3339)}
    for _, rid := range routes {
        s.Broker.Publish(rid, SSEEvent{Type: "pod.captured", Data: data})
    }
    writeJSON(w, http.StatusCreated, map[string]any{"podId": id, "status": status})
}

// SubscriptionsHandler handles POST /v1/subscriptions
func (s *Server) SubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        p := s.getPrincipal(r)
        if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
        var req model.SubscriptionRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeProblem(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
            return
        }
        sub, err := s.Store.CreateSubscription(r.Context(), req)
        if err != nil {
            writeProblem(w, http.StatusInternalServerError, "Create subscription failed", err.Error(), r.URL.Path)
            return
        }
        writeJSON(w, http.StatusCreated, sub)
    case http.MethodGet:
        // Admin list
        p := s.getPrincipal(r)
        if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
        cursor := r.URL.Query().Get("cursor")
        limit := 100
        if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
        items, next, err := s.Store.ListSubscriptions(r.Context(), p.Tenant, cursor, limit)
        if err != nil { writeProblem(w, 500, "List subscriptions failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

// ETAStreamHandler demonstrates SSE for ETA updates
func (s *Server) ETAStreamHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    routeID := r.URL.Query().Get("routeId")
    if routeID == "" {
        writeProblem(w, http.StatusBadRequest, "Missing routeId", "", r.URL.Path)
        return
    }
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    // Minimal demo event
    fmt.Fprintf(w, "event: eta.updated\n")
    fmt.Fprintf(w, "data: {\"routeId\":\"%s\",\"eta\":\"%s\"}\n\n", routeID, time.Now().Add(5*time.Minute).Format(time.RFC3339))
}

// DriversHandler handles shift and break endpoints under /v1/drivers/{driverId}/...
func (s *Server) DriversHandler(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(r.URL.Path, "/v1/drivers/") {
        writeProblem(w, http.StatusNotFound, "Not Found", "", r.URL.Path)
        return
    }
    rest := strings.TrimPrefix(r.URL.Path, "/v1/drivers/")
    parts := strings.Split(rest, "/")
    if len(parts) < 2 { writeProblem(w, http.StatusNotFound, "Not Found", "", r.URL.Path); return }
    driverID := parts[0]
    action := strings.Join(parts[1:], "/")

    var ts time.Time
    var note, breakType string
    if r.Method == http.MethodPost {
        // parse ts from body
        var body map[string]any
        if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
            if v, ok := body["ts"].(string); ok { t, _ := time.Parse(time.RFC3339, v); if !t.IsZero() { ts = t } }
            if v, ok := body["note"].(string); ok { note = v }
            if v, ok := body["type"].(string); ok { breakType = v }
        }
    }
    if ts.IsZero() { ts = time.Now().UTC() }
    _, tenant := s.withTenant(r)
    upd := model.HOSUpdate{Action: actionToHOSAction(action), TS: ts.Format(time.RFC3339), Type: breakType, Note: note}
    status, hos, err := s.Store.UpdateHOS(r.Context(), tenant, driverID, upd)
    if err != nil {
        writeProblem(w, http.StatusInternalServerError, "HOS update failed", err.Error(), r.URL.Path)
        return
    }
    // Broadcast break started/ended via SSE + webhooks for active routes
    if upd.Action == "break_start" || upd.Action == "break_end" {
        routes, _ := s.Store.ListActiveRoutesForDriver(r.Context(), tenant, driverID)
        evtType := "hos.break.started"
        if upd.Action == "break_end" { evtType = "hos.break.ended" }
        data := map[string]any{"driverId": driverID, "ts": upd.TS}
        for _, rid := range routes {
            d := map[string]any{"routeId": rid}
            for k,v := range data { d[k]=v }
            s.Broker.Publish(rid, SSEEvent{Type: evtType, Data: d})
            s.Pub.Emit(r.Context(), tenant, evtType, d)
        }
    }
    writeJSON(w, http.StatusOK, map[string]any{"driverId": driverID, "status": status, "hosState": hos})
}

func actionToHOSAction(path string) string {
    switch path {
    case "shift/start": return "shift_start"
    case "shift/end": return "shift_end"
    case "breaks/start": return "break_start"
    case "breaks/end": return "break_end"
    default: return path
    }
}

// Geofences
func (s *Server) GeofencesHandler(w http.ResponseWriter, r *http.Request) {
    _, tenant := s.withTenant(r)
    pr := s.getPrincipal(r)
    switch r.Method {
    case http.MethodGet:
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        cursor := r.URL.Query().Get("cursor")
        limit := 100
        if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
        items, next, err := s.Store.ListGeofences(r.Context(), tenant, cursor, limit)
        if err != nil { writeProblem(w, 500, "List geofences failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
    case http.MethodPost:
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        var in model.GeofenceInput
        if err := json.NewDecoder(r.Body).Decode(&in); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
        gf, err := s.Store.CreateGeofence(r.Context(), tenant, in)
        if err != nil { writeProblem(w, 500, "Create geofence failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 201, gf)
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

func (s *Server) GeofenceByIDHandler(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(r.URL.Path, "/v1/geofences/") { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    id := strings.TrimPrefix(r.URL.Path, "/v1/geofences/")
    _, tenant := s.withTenant(r)
    pr := s.getPrincipal(r)
    switch r.Method {
    case http.MethodGet:
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        gf, err := s.Store.GetGeofence(r.Context(), tenant, id)
        if err != nil { writeProblem(w, 404, "Not Found", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, gf)
    case http.MethodPatch:
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        var in model.GeofenceInput
        if err := json.NewDecoder(r.Body).Decode(&in); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
        gf, err := s.Store.PatchGeofence(r.Context(), tenant, id, in)
        if err != nil { writeProblem(w, 500, "Update geofence failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, gf)
    case http.MethodDelete:
        if !(pr.IsAdmin() || pr.Role == "dispatcher") { writeProblem(w, 403, "Forbidden", "dispatcher or admin required", r.URL.Path); return }
        if err := s.Store.DeleteGeofence(r.Context(), tenant, id); err != nil { writeProblem(w, 500, "Delete geofence failed", err.Error(), r.URL.Path); return }
        w.WriteHeader(204)
    default:
        w.WriteHeader(http.StatusMethodNotAllowed)
    }
}

// Media presign
func (s *Server) MediaPresignHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { w.WriteHeader(405); return }
    var in model.PresignRequest
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
    expire := time.Now().Add(15 * time.Minute).Format(time.RFC3339)
    writeJSON(w, 200, map[string]any{
        "uploadUrl": fmt.Sprintf("https://upload.example/%s?token=demo", in.FileName),
        "method": "PUT",
        "headers": map[string]string{"Content-Type": in.ContentType},
        "expireAt": expire,
    })
}

// Health
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) ReadyHandler(w http.ResponseWriter, r *http.Request) {
    // Check DB connectivity when using Postgres store
    type pinger interface{ Ping(ctx context.Context) error }
    if pg, ok := s.Store.(pinger); ok {
        ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
        defer cancel()
        if err := pg.Ping(ctx); err != nil { writeProblem(w, 503, "Not Ready", err.Error(), r.URL.Path); return }
    }
    writeJSON(w, 200, map[string]string{"status": "ready"})
}

// Subscription delete (admin)
func (s *Server) SubscriptionByIDHandler(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    if r.Method != http.MethodDelete { w.WriteHeader(405); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    id := strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")
    if err := s.Store.DeleteSubscription(r.Context(), p.Tenant, id); err != nil { writeProblem(w, 500, "Delete subscription failed", err.Error(), r.URL.Path); return }
    w.WriteHeader(204)
}

// Admin: webhook deliveries list and retry
func (s *Server) WebhookDeliveriesHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/webhook-deliveries" { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    if r.Method != http.MethodGet { w.WriteHeader(405); return }
    status := r.URL.Query().Get("status")
    cursor := r.URL.Query().Get("cursor")
    limit := 100
    if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
    items, next, err := s.Store.ListWebhookDeliveries(r.Context(), p.Tenant, status, cursor, limit)
    if err != nil { writeProblem(w, 500, "List deliveries failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

func (s *Server) WebhookDeliveryRetryHandler(w http.ResponseWriter, r *http.Request) {
    if !strings.HasPrefix(r.URL.Path, "/v1/admin/webhook-deliveries/") || !strings.HasSuffix(r.URL.Path, "/retry") { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    if r.Method != http.MethodPost { w.WriteHeader(405); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/admin/webhook-deliveries/"), "/retry")
    if err := s.Store.RetryWebhookDelivery(r.Context(), p.Tenant, id); err != nil { writeProblem(w, 500, "Retry delivery failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 202, map[string]int{"accepted": 1})
}

// Admin: webhook metrics
func (s *Server) WebhookMetricsHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/webhook-metrics" || r.Method != http.MethodGet { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    sinceHours := 24
    if v := r.URL.Query().Get("sinceHours"); v != "" { fmt.Sscanf(v, "%d", &sinceHours) }
    eventType := r.URL.Query().Get("eventType")
    status := r.URL.Query().Get("status")
    codeMin := 0; codeMax := 0
    if v := r.URL.Query().Get("responseCodeMin"); v != "" { fmt.Sscanf(v, "%d", &codeMin) }
    if v := r.URL.Query().Get("responseCodeMax"); v != "" { fmt.Sscanf(v, "%d", &codeMax) }
    // codeClass shorthand
    if v := r.URL.Query().Get("codeClass"); v != "" && codeMin == 0 && codeMax == 0 {
        switch v {
        case "2xx": codeMin, codeMax = 200, 299
        case "3xx": codeMin, codeMax = 300, 399
        case "4xx": codeMin, codeMax = 400, 499
        case "5xx": codeMin, codeMax = 500, 599
        }
    }
    // latency buckets from tenant config or defaults
    cfg, _ := s.Store.GetOptimizerConfig(r.Context(), p.Tenant)
    var buckets []int
    if cfg != nil {
        if lst, ok := cfg["latencyBuckets"].([]any); ok {
            for _, x := range lst { if f, ok := x.(float64); ok { buckets = append(buckets, int(f)) } }
        }
    }
    since := time.Now().Add(-time.Duration(sinceHours) * time.Hour)
    items, err := s.Store.WebhookMetrics(r.Context(), p.Tenant, since, eventType, status, codeMin, codeMax, buckets)
    if err != nil { writeProblem(w, 500, "Metrics failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 200, map[string]any{"items": items})
}

// Admin metrics: routes stats by planDate
func (s *Server) RouteStatsHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/routes/stats" || r.Method != http.MethodGet { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    planDate := r.URL.Query().Get("planDate")
    if planDate == "" { writeProblem(w, 400, "Missing planDate", "", r.URL.Path); return }
    stats, err := s.Store.RouteStats(r.Context(), p.Tenant, planDate)
    if err != nil { writeProblem(w, 500, "Stats failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 200, stats)
}

// Admin plan metrics by algorithm
func (s *Server) PlanMetricsHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/plan-metrics" || r.Method != http.MethodGet { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    planDate := r.URL.Query().Get("planDate")
    if planDate == "" { writeProblem(w, 400, "Missing planDate", "", r.URL.Path); return }
    algo := r.URL.Query().Get("algo")
    includeWeights := false
    if v := r.URL.Query().Get("includeWeights"); strings.EqualFold(v, "true") || v == "1" { includeWeights = true }
    // Prefer DB metrics; fallback to in-memory
    items, err := s.Store.ListPlanMetrics(r.Context(), p.Tenant, planDate, algo)
    if err != nil || len(items) == 0 {
        ms := opt.GetMetrics(p.Tenant, planDate)
        i2 := []map[string]any{}
        for a, m := range ms {
            if algo != "" && a != algo { continue }
            i2 = append(i2, map[string]any{
                "algo": a,
                "iterations": m.Iterations,
                "improvements": m.Improvements,
                "acceptedWorse": m.AcceptedWorse,
                "bestCost": m.BestCost,
                "finalCost": m.FinalCost,
                "removalSelects": []int{m.RemovalSelects[0], m.RemovalSelects[1]},
                "insertSelects": []int{m.InsertSelects[0], m.InsertSelects[1]},
            })
        }
        items = i2
    }
    if includeWeights {
        // attach weight snapshots per algo
        for i := range items {
            a, _ := items[i]["algo"].(string)
            if a == "" { continue }
            wsnaps, err := s.Store.ListPlanMetricsWeights(r.Context(), p.Tenant, planDate, a)
            if err == nil && len(wsnaps) > 0 { items[i]["weights"] = wsnaps }
        }
    }
    writeJSON(w, 200, map[string]any{"items": items})
}

// Admin plan metrics weight snapshots
func (s *Server) PlanMetricsWeightsHandler(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/v1/admin/plan-metrics/weights" || r.Method != http.MethodGet { writeProblem(w, 404, "Not Found", "", r.URL.Path); return }
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    planDate := r.URL.Query().Get("planDate")
    algo := r.URL.Query().Get("algo")
    if planDate == "" || algo == "" { writeProblem(w, 400, "Missing parameters", "planDate and algo required", r.URL.Path); return }
    items, err := s.Store.ListPlanMetricsWeights(r.Context(), p.Tenant, planDate, algo)
    if err != nil { writeProblem(w, 500, "Metrics weights failed", err.Error(), r.URL.Path); return }
    writeJSON(w, 200, map[string]any{"items": items})
}
// Admin: webhook DLQ list and requeue
func (s *Server) WebhookDLQHandler(w http.ResponseWriter, r *http.Request) {
    p := s.getPrincipal(r)
    if !p.IsAdmin() { writeProblem(w, 403, "Forbidden", "admin required", r.URL.Path); return }
    if r.URL.Path == "/v1/admin/webhook-dlq" && r.Method == http.MethodGet {
        cursor := r.URL.Query().Get("cursor")
        limit := 100
        if v := r.URL.Query().Get("limit"); v != "" { fmt.Sscanf(v, "%d", &limit) }
        eventType := r.URL.Query().Get("eventType")
        olderThanHours := 0
        if v := r.URL.Query().Get("olderThanHours"); v != "" { fmt.Sscanf(v, "%d", &olderThanHours) }
        var older time.Time
        if olderThanHours > 0 { older = time.Now().Add(-time.Duration(olderThanHours) * time.Hour) }
        codeMin := 0; codeMax := 0
        if v := r.URL.Query().Get("responseCodeMin"); v != "" { fmt.Sscanf(v, "%d", &codeMin) }
        if v := r.URL.Query().Get("responseCodeMax"); v != "" { fmt.Sscanf(v, "%d", &codeMax) }
        errorQuery := r.URL.Query().Get("errorQuery")
        items, next, err := s.Store.ListWebhookDLQ(r.Context(), p.Tenant, eventType, older, codeMin, codeMax, errorQuery, cursor, limit)
        if err != nil { writeProblem(w, 500, "List DLQ failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
        return
    }
    if r.URL.Path == "/v1/admin/webhook-dlq" && r.Method == http.MethodPost {
        var req struct{ IDs []string `json:"ids"` }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
        if len(req.IDs) == 0 { writeProblem(w, 400, "Missing ids", "", r.URL.Path); return }
        if err := s.Store.RequeueWebhookDLQBulk(r.Context(), p.Tenant, req.IDs); err != nil { writeProblem(w, 500, "Bulk requeue failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 202, map[string]int{"accepted": len(req.IDs)})
        return
    }
    if r.URL.Path == "/v1/admin/webhook-dlq" && r.Method == http.MethodDelete {
        var req struct{ IDs []string `json:"ids"`; OlderThanHours int `json:"olderThanHours"` }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeProblem(w, 400, "Invalid JSON", err.Error(), r.URL.Path); return }
        var older time.Time
        if req.OlderThanHours > 0 { older = time.Now().Add(-time.Duration(req.OlderThanHours) * time.Hour) }
        if err := s.Store.DeleteWebhookDLQBulk(r.Context(), p.Tenant, req.IDs, older); err != nil { writeProblem(w, 500, "Bulk delete failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 202, map[string]int{"accepted": 1})
        return
    }
    if strings.HasPrefix(r.URL.Path, "/v1/admin/webhook-dlq/") && strings.HasSuffix(r.URL.Path, "/requeue") && r.Method == http.MethodPost {
        id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/admin/webhook-dlq/"), "/requeue")
        if err := s.Store.RequeueWebhookDLQ(r.Context(), p.Tenant, id); err != nil { writeProblem(w, 500, "Requeue failed", err.Error(), r.URL.Path); return }
        writeJSON(w, 202, map[string]int{"accepted": 1})
        return
    }
    writeProblem(w, 404, "Not Found", "", r.URL.Path)
}
