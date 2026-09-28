-- +goose Up
-- +goose StatementBegin

-- Durable "the daemon has handed this commit to the remote" fact.
--
-- A git commit is not a SQLite change, so it cannot reach change_log through a
-- trigger. The head observer reads each live session's worktree and records what
-- it delivered, so the next poll can tell a commit it already acted on from a new
-- one. The column is deliberately named for what it records -- a delivery, not an
-- observation -- because that narrower meaning is what makes the trigger
-- self-limiting: a successful attempt writes the head and stops, a failed one
-- leaves it empty and stays retryable.
--
-- It is a column rather than a new event type because the fact has to survive a
-- daemon restart. Without it every boot would re-attempt delivery for every
-- committed session, which is at best a redundant `gh pr list` and at worst a
-- second push for a branch that already diverged.
--
-- DEFAULT '' is load-bearing: every row written before this migration had nothing
-- delivered, and every synthetic SessionRecord in the codebase has Go's zero
-- value, so existing behaviour is preserved with no backfill.
--
-- `sessions_cdc_update` is intentionally NOT extended with this column. Nothing
-- user-visible changes when the head moves -- the board column is derived from
-- daemon-observed PR facts only, never from a commit -- and the delivery leg that
-- does change the card announces itself through the new pull request's own change
-- events. Firing here would only churn every connected board.
ALTER TABLE sessions ADD COLUMN delivered_head_sha TEXT NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- SQLite's DROP COLUMN arrived in 3.35; the daemon pins modernc.org/sqlite
-- v1.51.0, well past it. No other table references this column, so the drop needs
-- no table rebuild, no index changes, and no trigger rewrite.
ALTER TABLE sessions DROP COLUMN delivered_head_sha;

-- +goose StatementEnd
