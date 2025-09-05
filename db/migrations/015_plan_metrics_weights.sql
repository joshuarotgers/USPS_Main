CREATE TABLE IF NOT EXISTS plan_metrics_weights (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  plan_date date NOT NULL,
  algo text NOT NULL,
  iteration int NOT NULL,
  removal_weights jsonb NOT NULL,
  insertion_weights jsonb NOT NULL,
  created_at timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pm_weights_tenant_date_algo ON plan_metrics_weights(tenant_id, plan_date, algo);
