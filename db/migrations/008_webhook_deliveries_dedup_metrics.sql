ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS dedup_key text;
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS delivered_at timestamptz;
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS response_code int;
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS latency_ms int;

CREATE UNIQUE INDEX IF NOT EXISTS ux_wd_dedup ON webhook_deliveries(tenant_id, event_type, url, dedup_key);
