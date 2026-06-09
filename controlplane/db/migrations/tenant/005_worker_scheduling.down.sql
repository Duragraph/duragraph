DROP INDEX IF EXISTS idx_runs_worker;
DROP INDEX IF EXISTS idx_runs_claim;
ALTER TABLE runs DROP COLUMN IF EXISTS graph_id;
ALTER TABLE runs DROP COLUMN IF EXISTS priority;
ALTER TABLE runs DROP COLUMN IF EXISTS worker_id;
DROP TABLE IF EXISTS crons;
DROP TABLE IF EXISTS workers;
-- Last tenant migration: drop the shared trigger function.
DROP FUNCTION IF EXISTS update_updated_at_column();
