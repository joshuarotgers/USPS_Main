CREATE TABLE IF NOT EXISTS webhook_dlq (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  delivery_id uuid,
  event_type text NOT NULL,
  url text NOT NULL,
  secret text,
  payload jsonb NOT NULL,
  attempts int NOT NULL,
  last_error text,
  created_at timestamptz DEFAULT now(),
  updated_at timestamptz DEFAULT now()
);

ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS max_attempts int DEFAULT 10;
