-- +goose Up
-- The typed task a run executed (docs/10 phase 3, the agent's task_run):
-- the plan, the task's key in it, its type and label, as the agent reported
-- the run; NULL for a plugin's run. The task's termination is never stored
-- here: the server proves it from the ingest when asked (GET
-- /api/microbot/task).
ALTER TABLE routine_runs ADD COLUMN task JSONB
  CONSTRAINT routine_runs_task_object CHECK (task IS NULL OR jsonb_typeof(task) = 'object');
COMMENT ON COLUMN routine_runs.task IS 'The typed task of a task run (agent.RunTask: plan, key, type, label); NULL for a plugin run.';

-- +goose Down
ALTER TABLE routine_runs DROP COLUMN task;
