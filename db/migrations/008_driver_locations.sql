-- Latest driver locations per tenant/route/driver
CREATE TABLE IF NOT EXISTS driver_locations_latest (
  tenant_id uuid NOT NULL,
  route_id uuid NOT NULL,
  driver_id text NOT NULL,
  lat double precision NOT NULL,
  lng double precision NOT NULL,
  ts timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, route_id, driver_id)
);

CREATE INDEX IF NOT EXISTS idx_driver_locations_route ON driver_locations_latest (tenant_id, route_id);

