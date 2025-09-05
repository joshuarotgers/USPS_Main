ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS final_removal_weights jsonb;
ALTER TABLE plan_metrics ADD COLUMN IF NOT EXISTS final_insertion_weights jsonb;
