-- NULL preserves legacy plans: all eight customer management modules enabled.
-- Explicit settings are validated by plan CRUD and on every authorization read.
ALTER TABLE service_plans ADD COLUMN IF NOT EXISTS features TEXT NULL;
