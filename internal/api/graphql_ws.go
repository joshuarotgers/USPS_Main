package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Minimal GraphQL over WebSocket (graphql-transport-ws like) to stream routeEvents

var upgrader = websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

type wsMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type subscribePayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// GraphQLWSHandler handles /graphql/ws
func (s *Server) GraphQLWSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
    defer func() { _ = conn.Close() }()

	// Track subscriptions: id -> routeID and channel
	type sub struct {
		routeID string
		ch      chan SSEEvent
	}
	subs := map[string]sub{}

	// Read loop
	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { _ = conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	// Write helper
	write := func(v any) error { return conn.WriteJSON(v) }

	// Expect connection_init first
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		switch msg.Type {
		case "connection_init":
			// Acknowledge
			_ = write(wsMessage{Type: "connection_ack"})
			// Start keepalive
			go func() {
				ticker := time.NewTicker(20 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if err := write(wsMessage{Type: "ping"}); err != nil {
						return
					}
				}
			}()
		case "ping":
			_ = write(wsMessage{Type: "pong"})
		case "subscribe":
			// Parse subscription payload and set up route event stream
			var pl subscribePayload
			_ = json.Unmarshal(msg.Payload, &pl)
			rid := ""
			if pl.Variables != nil {
				if v, ok := pl.Variables["routeId"].(string); ok {
					rid = v
				}
			}
			if rid == "" {
				// crude parse fallback
            if strings.Contains(pl.Query, "routeEvents") {
                // require routeId variable; if missing, send error
                _ = write(wsMessage{Type: "error", ID: msg.ID, Payload: []byte(`{"message":"routeId required"}`)})
                _ = write(wsMessage{Type: "complete", ID: msg.ID})
                continue
            }
			}
			// RBAC: admin/dispatcher or assigned driver
			pr := s.getPrincipal(r)
			if !(pr.IsAdmin() || pr.Role == "dispatcher") {
				_, tenant := s.withTenant(r)
				rt, err := s.Store.GetRoute(r.Context(), tenant, rid)
				if err != nil || pr.Role != "driver" || pr.DriverID == "" || rt.DriverID == "" || pr.DriverID != rt.DriverID {
					// send error and complete
					_ = write(wsMessage{Type: "error", ID: msg.ID, Payload: []byte(`{"message":"forbidden"}`)})
					_ = write(wsMessage{Type: "complete", ID: msg.ID})
					continue
				}
			}
			// determine requested field: routeEvents, policyAlerts, podCaptured, breakEvents
			field := "routeEvents"
			ql := strings.ToLower(pl.Query)
			if strings.Contains(ql, "policyalerts") {
				field = "policyAlerts"
			}
			if strings.Contains(ql, "podcaptured") {
				field = "podCaptured"
			}
			if strings.Contains(ql, "breakevents") {
				field = "breakEvents"
			}
			ch := s.Broker.Subscribe(rid)
			subs[msg.ID] = sub{routeID: rid, ch: ch}
			// Fanout goroutine
			go func(id string, c chan SSEEvent, field string) {
				for evt := range c {
					// Filter by field when needed
					if field == "policyAlerts" && evt.Type != "policy.alert" {
						continue
					}
					if field == "podCaptured" && evt.Type != "pod.captured" {
						continue
					}
					if field == "breakEvents" && !(strings.HasPrefix(evt.Type, "hos.break.")) {
						continue
					}
					key := field
					if field == "routeEvents" {
						key = "routeEvents"
					}
					data := map[string]any{key: evt.Data}
					payload, _ := json.Marshal(map[string]any{"data": data})
					_ = write(wsMessage{Type: "next", ID: id, Payload: payload})
				}
				_ = write(wsMessage{Type: "complete", ID: id})
			}(msg.ID, ch, field)
		case "complete":
			if s0, ok := subs[msg.ID]; ok {
				s.Broker.Unsubscribe(s0.routeID, s0.ch)
				delete(subs, msg.ID)
			}
		default:
			// ignore
		}
	}
	// Cleanup
	for id, s0 := range subs {
		s.Broker.Unsubscribe(s0.routeID, s0.ch)
		delete(subs, id)
	}
}
