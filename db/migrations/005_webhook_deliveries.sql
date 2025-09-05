CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  subscription_id uuid REFERENCES subscriptions(id) ON DELETE SET NULL,
  event_type text NOT NULL,
  url text NOT NULL,
  secret text,
  payload jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  attempts int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz DEFAULT now(),
  last_error text,
  created_at timestamptz DEFAULT now(),
  updated_at timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wd_tenant_status_next ON webhook_deliveries (tenant_id, status, next_attempt_at);
