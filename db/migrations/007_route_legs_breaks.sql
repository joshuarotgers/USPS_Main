ALTER TABLE route_legs ADD COLUMN IF NOT EXISTS kind text;
ALTER TABLE route_legs ADD COLUMN IF NOT EXISTS break_sec int;
-- Optional: default existing rows to 'drive'
UPDATE route_legs SET kind = COALESCE(kind, 'drive');
