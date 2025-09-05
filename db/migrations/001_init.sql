-- Initial Postgres schema (multi-tenant)
-- NOTE: Enable required extensions in your environment as needed (e.g., postgis, pgcrypto, citext)

CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  region text,
  settings jsonb DEFAULT '{}'::jsonb,
  created_at timestamptz DEFAULT now()
);

CREATE TABLE users (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  role text NOT NULL,
  email text,
  locale text,
  status text,
  created_at timestamptz DEFAULT now()
);

CREATE TABLE drivers (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  license text,
  skills text[],
  shift_window tstzrange,
  hos_state jsonb DEFAULT '{}'::jsonb
);

CREATE TABLE vehicles (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  type text,
  capacity jsonb,
  dims jsonb,
  restrictions jsonb,
  attrs jsonb
);

CREATE TABLE orders (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  external_ref text,
  priority int DEFAULT 0,
  service_level text,
  status text,
  attrs jsonb,
  created_at timestamptz DEFAULT now(),
  UNIQUE (tenant_id, external_ref)
);

CREATE TABLE stops (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  order_id uuid REFERENCES orders(id) ON DELETE CASCADE,
  type text NOT NULL,
  address text,
  -- location geography(point,4326) -- uncomment if PostGIS is available
  lat double precision,
  lng double precision,
  time_window tstzrange,
  service_time_sec int DEFAULT 0,
  geofence_id uuid,
  required_skills text[],
  status text
);

CREATE TABLE routes (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  version int NOT NULL DEFAULT 1,
  plan_date date NOT NULL,
  depot_id text,
  vehicle_id uuid REFERENCES vehicles(id),
  driver_id uuid REFERENCES drivers(id),
  status text,
  cost_breakdown jsonb,
  created_by uuid,
  locked_until timestamptz
);

CREATE TABLE route_legs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  route_id uuid NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
  seq int NOT NULL,
  from_stop_id uuid REFERENCES stops(id),
  to_stop_id uuid REFERENCES stops(id),
  dist_m int,
  drive_sec int,
  eta_arrival timestamptz,
  eta_departure timestamptz,
  status text
);

CREATE TABLE constraints (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  type text NOT NULL,
  payload jsonb NOT NULL,
  active boolean DEFAULT true
);

CREATE TABLE geofences (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  -- polygon geography(polygon,4326), -- uncomment if PostGIS is available
  radius_m int,
  type text,
  rules jsonb
);

CREATE TABLE events (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  type text NOT NULL,
  entity_id uuid,
  ts timestamptz NOT NULL,
  payload jsonb,
  source text
);

CREATE TABLE pods (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  order_id uuid REFERENCES orders(id) ON DELETE SET NULL,
  stop_id uuid REFERENCES stops(id) ON DELETE SET NULL,
  type text NOT NULL,
  media_url text,
  hash text,
  ts timestamptz DEFAULT now(),
  by_driver_id uuid REFERENCES drivers(id),
  metadata jsonb
);

-- Indexing
CREATE INDEX idx_routes_tenant_date ON routes (tenant_id, plan_date);
CREATE INDEX idx_orders_tenant_status ON orders (tenant_id, status);
CREATE INDEX idx_stops_tenant_status ON stops (tenant_id, status);
CREATE INDEX idx_events_tenant_ts ON events (tenant_id, ts DESC);
CREATE INDEX idx_constraints_tenant_active ON constraints (tenant_id, active);

-- Optional RLS example (enable per table in production)
-- ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
-- CREATE POLICY tenant_isolation_orders ON orders USING (tenant_id = current_setting('app.tenant_id')::uuid);

