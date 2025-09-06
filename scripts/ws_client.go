// Package main runs a demo WebSocket client for route events.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	base := fmt.Sprintf("http://localhost:%s", port)

	// Create a simple route via optimize
	body := []byte(`{"tenantId":"t_demo","planDate":"2024-09-05","algorithm":"greedy"}`)
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/optimize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-Id", "t_demo")
	req.Header.Set("X-Role", "admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var optResp struct {
		Routes []struct {
			ID string `json:"id"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&optResp); err != nil {
		log.Fatal(err)
	}
	if len(optResp.Routes) == 0 {
		log.Fatal("no routes returned")
	}
	routeID := optResp.Routes[0].ID
	log.Printf("Route ID: %s", routeID)

	// Connect WS
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/graphql/ws"}
	hdr := http.Header{}
	hdr.Set("X-Tenant-Id", "t_demo")
	hdr.Set("X-Role", "admin")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer func() { _ = c.Close() }()

	// connection_init
	if err := c.WriteJSON(wsMessage{Type: "connection_init"}); err != nil {
		log.Fatal(err)
	}
	// subscribe to routeEvents
	payload := map[string]any{
		"query":     "subscription($routeId: ID!) { routeEvents(routeId: $routeId) }",
		"variables": map[string]any{"routeId": routeID},
	}
	pl, _ := json.Marshal(payload)
	if err := c.WriteJSON(wsMessage{Type: "subscribe", ID: "1", Payload: pl}); err != nil {
		log.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var m wsMessage
			if err := c.ReadJSON(&m); err != nil {
				log.Printf("read: %v", err)
				return
			}
			log.Printf("WS <- %s: %s", m.Type, string(m.Payload))
		}
	}()

	// Trigger a route event via advance
	time.Sleep(500 * time.Millisecond)
	advReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/routes/%s/advance", base, routeID), bytes.NewReader([]byte("{}")))
	advReq.Header.Set("Content-Type", "application/json")
	advReq.Header.Set("X-Tenant-Id", "t_demo")
	advReq.Header.Set("X-Role", "admin")
	_, _ = http.DefaultClient.Do(advReq)

	// Wait briefly to receive a few messages
	select {
	case <-time.After(2 * time.Second):
	case <-done:
	}
}
