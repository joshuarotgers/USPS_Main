-- Offline-first local SQLite schema for driver app

CREATE TABLE IF NOT EXISTS local_routes (
  route_id TEXT PRIMARY KEY,
  version INTEGER NOT NULL,
  status TEXT,
  plan_date TEXT,
  meta_json TEXT
);

CREATE TABLE IF NOT EXISTS local_stops (
  stop_id TEXT PRIMARY KEY,
  route_id TEXT NOT NULL,
  seq INTEGER,
  status TEXT,
  address TEXT,
  lat REAL,
  lng REAL,
  tw_start TEXT,
  tw_end TEXT,
  service_time_sec INTEGER,
  payload_json TEXT,
  FOREIGN KEY(route_id) REFERENCES local_routes(route_id)
);

CREATE TABLE IF NOT EXISTS outbox_events (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  route_id TEXT,
  stop_id TEXT,
  ts TEXT NOT NULL,
  payload_json TEXT,
  idem_key TEXT,
  attempts INTEGER DEFAULT 0,
  last_error TEXT
);

CREATE TABLE IF NOT EXISTS media_queue (
  id TEXT PRIMARY KEY,
  stop_id TEXT,
  type TEXT,
  file_path TEXT,
  sha256 TEXT,
  status TEXT,
  upload_url TEXT,
  attempts INTEGER DEFAULT 0,
  last_error TEXT
);

CREATE TABLE IF NOT EXISTS kv (
  key TEXT PRIMARY KEY,
  value TEXT
);

CREATE TABLE IF NOT EXISTS clock (
  server_offset_ms INTEGER
);

