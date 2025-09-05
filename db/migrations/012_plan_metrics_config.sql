ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS init_temp double precision;
ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS cooling double precision;
ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS init_removal_weights jsonb;
ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS init_insertion_weights jsonb;
