CREATE TABLE IF NOT EXISTS optimizer_config (
  tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
  config jsonb NOT NULL,
  updated_at timestamptz DEFAULT now()
);

