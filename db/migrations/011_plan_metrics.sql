CREATE TABLE IF NOT EXISTS plan_metrics (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  plan_date date NOT NULL,
  algo text NOT NULL,
  iterations int NOT NULL DEFAULT 0,
  improvements int NOT NULL DEFAULT 0,
  accepted_worse int NOT NULL DEFAULT 0,
  best_cost double precision,
  final_cost double precision,
  removal_selects jsonb,
  insert_selects jsonb,
  created_at timestamptz DEFAULT now(),
  UNIQUE (tenant_id, plan_date, algo)
);

CREATE INDEX IF NOT EXISTS idx_plan_metrics_tenant_date ON plan_metrics(tenant_id, plan_date);
