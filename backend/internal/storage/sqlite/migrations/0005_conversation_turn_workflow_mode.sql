-- +goose Up
-- +goose StatementBegin

-- Durable send-mode for human prompts: the workflow stage (planning, manager,
-- or building) that was active when a turn was sent. The conversation-level
-- mode is current-only, so without this column the mode-at-send cannot be
-- derived after a mode switch, and the timeline could not color prompts by the
-- mode they went out in.
--
-- DEFAULT '' is load-bearing: rows written before this migration (and turns the
-- provider started, which carry no send mode) have no recorded workflow, and
-- the renderer draws those with no mode edge rather than guessing the current
-- mode. CHECK keeps the column to the stages the daemon resolves; the empty
-- legacy value is the only other legal state.
ALTER TABLE conversation_turns ADD COLUMN workflow_mode TEXT NOT NULL DEFAULT ''
    CHECK (workflow_mode IN ('', 'planning', 'manager', 'building'));

-- The edit-delivery row caches the replacement turn it accepted, so an
-- idempotent replay can return the same result without re-dispatching. That
-- cached copy carries the replacement's recorded send-mode with it.
ALTER TABLE conversation_edit_deliveries ADD COLUMN turn_workflow_mode TEXT NOT NULL DEFAULT ''
    CHECK (turn_workflow_mode IN ('', 'planning', 'manager', 'building'));

-- +goose StatementEnd
