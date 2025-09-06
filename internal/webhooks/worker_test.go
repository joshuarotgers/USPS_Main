package webhooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gpsnav/internal/store"
)

type recordStore struct {
	*store.Memory
	mu    sync.Mutex
	marks []MarkRec
	fails []FailRec
}
type MarkRec struct {
	ID            string
	Success       bool
	Code, Latency int
	LastErr       string
}
type FailRec struct {
	ID            string
	Code, Latency int
	LastErr       string
}

func (r *recordStore) MarkWebhookDelivery(ctx context.Context, id string, success bool, nextAttemptAt *time.Time, lastError string, responseCode int, latencyMs int) error {
	r.mu.Lock()
	r.marks = append(r.marks, MarkRec{ID: id, Success: success, Code: responseCode, Latency: latencyMs, LastErr: lastError})
	r.mu.Unlock()
	return r.Memory.MarkWebhookDelivery(ctx, id, success, nextAttemptAt, lastError, responseCode, latencyMs)
}
func (r *recordStore) FailWebhookDelivery(ctx context.Context, id string, lastError string, responseCode int, latencyMs int) error {
	r.mu.Lock()
	r.fails = append(r.fails, FailRec{ID: id, Code: responseCode, Latency: latencyMs, LastErr: lastError})
	r.mu.Unlock()
	return r.Memory.FailWebhookDelivery(ctx, id, lastError, responseCode, latencyMs)
}

func TestWorkerProcessOnce_SuccessAndSignature(t *testing.T) {
	var gotSig, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Signature")
		gotType = r.Header.Get("X-Event-Type")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rs := &recordStore{Memory: store.NewMemory()}
	w := &Worker{Store: rs, HTTP: srv.Client(), Stop: make(chan struct{}), MaxAttempts: 3}
    id, err := rs.Memory.EnqueueWebhook(context.Background(), "t1", "", "stop.advanced", srv.URL, "secret", []byte(`{"id":"evt1"}`))
	if err != nil || id == "" {
		t.Fatalf("enqueue failed: %v", err)
	}

	w.processOnce()

	if gotSig == "" || gotType != "stop.advanced" {
		t.Fatalf("missing signature/type headers: sig=%q type=%q", gotSig, gotType)
	}
	if len(rs.marks) == 0 || !rs.marks[0].Success {
		t.Fatalf("expected mark success, got: %+v", rs.marks)
	}
}

func TestWorkerProcessOnce_Fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	rs := &recordStore{Memory: store.NewMemory()}
	w := &Worker{Store: rs, HTTP: srv.Client(), Stop: make(chan struct{}), MaxAttempts: 1}
    _, _ = rs.Memory.EnqueueWebhook(context.Background(), "t1", "", "stop.advanced", srv.URL, "", []byte(`{}`))
	w.processOnce()
	if len(rs.fails) == 0 {
		t.Fatalf("expected fail recorded")
	}
}
