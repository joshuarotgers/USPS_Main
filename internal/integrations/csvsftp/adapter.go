package csvsftp

import (
    "strings"

    "gpsnav/internal/integrations"
)

// CsvSftpAdapter is a minimal example adapter parsing CSV orders fetched via SFTP.
type CsvSftpAdapter struct{}

func (a CsvSftpAdapter) Name() string { return "csv-sftp" }

func (a CsvSftpAdapter) Authenticate(cfg map[string]any) (integrations.AuthState, error) {
    return integrations.AuthState{Method: "sftp", Token: "keyref://example"}, nil
}

func (a CsvSftpAdapter) FetchOrders(since string, cursor string) (integrations.OrderBatch, error) {
    // Placeholder: in real impl, list files by mtime > since, parse CSV -> orders
    return integrations.OrderBatch{Orders: []integrations.Order{{ExternalRef: "CSV-1", Priority: 1}}}, nil
}

func (a CsvSftpAdapter) AckOrders(ids []string) error { return nil }

func (a CsvSftpAdapter) MapStatus(ext integrations.ExternalStatus) integrations.InternalEvent {
    typ := "created"
    if strings.EqualFold(ext.Code, "DELIVERED") {
        typ = "delivered"
    }
    return integrations.InternalEvent{Type: typ, Payload: map[string]any{"code": ext.Code}}
}

func (a CsvSftpAdapter) Webhooks() integrations.WebhookInfo {
    return integrations.WebhookInfo{Events: []string{}, Verify: func(sig string, body []byte) bool { return true }}
}

