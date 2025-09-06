package store

import (
	"encoding/hex"
	"testing"
)

func TestComputeDedupKeyFromID(t *testing.T) {
	body := []byte(`{"id":"evt_123","type":"x"}`)
	got := computeDedupKey(body)
	if got != "evt_123" {
		t.Fatalf("want evt_123, got %s", got)
	}
}

func TestComputeDedupKeyFromHash(t *testing.T) {
	body := []byte(`{"notId":"x"}`)
	got := computeDedupKey(body)
	// hex-encoded first 8 bytes -> 16 hex chars
	b, err := hex.DecodeString(got)
	if err != nil {
		t.Fatalf("invalid hex: %v", err)
	}
	if len(b) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(b))
	}
}

func TestPQStringArray(t *testing.T) {
	if v := pqStringArray(nil); v != nil {
		t.Fatalf("nil slice -> nil expected")
	}
	if v := pqStringArray([]string{}); v != nil {
		t.Fatalf("empty slice -> nil expected")
	}
	if v := pqStringArray([]string{"a", "b"}); v == nil {
		t.Fatalf("non-empty -> non-nil expected")
	}
}
