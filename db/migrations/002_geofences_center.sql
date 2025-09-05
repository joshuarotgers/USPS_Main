ALTER TABLE geofences ADD COLUMN IF NOT EXISTS name text;
ALTER TABLE geofences ADD COLUMN IF NOT EXISTS lat double precision;
ALTER TABLE geofences ADD COLUMN IF NOT EXISTS lng double precision;
