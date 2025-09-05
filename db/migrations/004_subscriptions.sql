CREATE TABLE IF NOT EXISTS subscriptions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  url text NOT NULL,
  events jsonb NOT NULL,
  secret text,
  created_at timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subs_tenant ON subscriptions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_subs_events ON subscriptions USING GIN (events);
