-- +goose Up
-- +goose StatementBegin

-- Durable re-engagement state for idle manager sessions.
--
-- Restores the state that migration 0001 never carried: the original feature
-- (79a70e82f) shipped with a 0038 that was reverted by ef4d6c124 before the
-- migration chain was retired and folded into the 0001 baseline. The table is
-- back because a manager that stalls needs a wake-up that survives daemon
-- restarts, and because "this manager is finished" has to be durable rather
-- than inferred from a transcript.
CREATE TABLE manager_reengagements (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMP NOT NULL,
    last_attempt_at TIMESTAMP,
    progress_since_attempt BOOLEAN NOT NULL DEFAULT 0,
    attention_notified BOOLEAN NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'completed', 'exhausted')),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_manager_reengagements_due
    ON manager_reengagements(state, next_attempt_at)
    WHERE state = 'active';

CREATE INDEX idx_manager_reengagements_attention
    ON manager_reengagements(state, attention_notified)
    WHERE state = 'exhausted' AND attention_notified = 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_manager_reengagements_due;
DROP INDEX IF EXISTS idx_manager_reengagements_attention;
DROP TABLE IF EXISTS manager_reengagements;
-- +goose StatementEnd
