-- +goose Up
-- +goose StatementBegin

-- Durable "the plan behind this work was reviewed" fact.
--
-- The worker stage discipline reviews the plan while it is still uncommitted
-- and lets the commit forward the card toward Ready. The explicit
-- planning-to-building stage transition is the recorded approval, and the head
-- observer hands an agent's own commit to the remote only once that approval
-- exists -- otherwise commit-before-review would earn an instant pull request.
-- The flag is set and cleared atomically with the workflow-mode change itself
-- (see SetSessionWorkflowMode), never by full-row replays, for the same reason
-- delivered_head_sha is excluded from InsertSession/UpdateSession: replaying a
-- record read before the fact was observed would otherwise clear a real review.
--
-- DEFAULT 0 is load-bearing for new rows: every worker starts in planning,
-- which is never an approved posture. The backfill below covers rows that are
-- already building: their transition predates this fact, and stranding live
-- work that was approved under the old regime would be hostile, so a building
-- worker is assumed reviewed. A worker sent back to planning (or a manager,
-- which never leaves its own stage) carries no approval.
--
-- `sessions_cdc_update` is intentionally NOT extended: the stage transition
-- that flips this flag already announces itself through the workflow_mode
-- condition, so a connected board learns about the approval with no extra
-- churn.
ALTER TABLE sessions ADD COLUMN plan_approved BOOLEAN NOT NULL DEFAULT 0;

UPDATE sessions SET plan_approved = 1 WHERE kind = 'worker' AND workflow_mode = 'building';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- SQLite's DROP COLUMN arrived in 3.35; the daemon pins modernc.org/sqlite
-- v1.51.0, well past it. No other table references this column, so the drop needs
-- no table rebuild, no index changes, and no trigger rewrite.
ALTER TABLE sessions DROP COLUMN plan_approved;

-- +goose StatementEnd
