ALTER TABLE webhook_dlq ADD COLUMN IF NOT EXISTS response_code int;
ALTER TABLE webhook_dlq ADD COLUMN IF NOT EXISTS latency_ms int;
