-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
-- Retiring a session deletes its row, and three things had to be made safe
-- before that could exist at all.
--
-- 1. Session numbers must never come back. num was allocated as MAX(num)+1, so
--    deleting a row freed its number and the next spawn could be handed the id
--    that a change_log row, a registered worktree path, or a PR conversation
--    still referenced. Retired numbers are recorded so allocation skips them.
--    A standalone session has no project, and NULL cannot join in a PRIMARY KEY,
--    so the project is stored as the empty string.
--
-- 2. A delete has to be visible. sessions had insert and update CDC triggers
--    only, so a deleted row left every connected client showing a card for a
--    session that no longer existed until a full refetch.
--
-- 3. A manager's conversation turns must survive it. handled_by_session_id was
--    NOT NULL ON DELETE CASCADE, so retiring a manager destroyed the turns it
--    handled while their messages and activities survived as orphans on SET
--    NULL -- the manager's own history silently gutted, and the project
--    narrative left with holes. The column is nullable now and a retired owner
--    detaches rather than cascading. The rebuild follows the same shape as
--    migration 0118, including every index and trigger, because SQLite cannot
--    alter a column's nullability in place.
PRAGMA foreign_keys=OFF;

CREATE TABLE retired_session_nums (
    project_id TEXT NOT NULL,
    num        INTEGER NOT NULL,
    retired_at TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, num)
);

-- A delete emits the same event type an update does, and for the same reason
-- the client acts on it: session_updated invalidates the workspace query, so the
-- refetch that follows simply no longer contains the row and the card goes. A
-- new event type would mean widening change_log's CHECK and teaching the client
-- a second delete path for no behavioural gain.
-- session_id is deliberately NULL: the trigger runs after the row is gone, and
-- an immediate foreign key cannot reference it. The id travels in the payload,
-- which is what the client reads. change_log.session_id being nullable is what
-- makes a delete event representable at all.
CREATE TRIGGER sessions_cdc_delete
AFTER DELETE ON sessions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (OLD.project_id, NULL, 'session_updated',
        json_object('id', OLD.id, 'sessionId', OLD.id, 'retired', json('true'),
                    'kind', COALESCE(OLD.kind, 'worker'), 'num', OLD.num),
        datetime('now'));
END;

DROP TRIGGER IF EXISTS conversation_turns_cdc_update;
DROP TRIGGER IF EXISTS conversation_turns_branch_insert;

CREATE TABLE conversation_turns_next (
    id                     TEXT PRIMARY KEY,
    conversation_id        TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    -- The one change: a retired session detaches its turns instead of taking
    -- them with it. Turns that keep an owner are unchanged.
    handled_by_session_id  TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    provider_turn_id       TEXT NOT NULL DEFAULT '',
    controller_generation  TEXT NOT NULL DEFAULT '',
    state                  TEXT NOT NULL CHECK (state IN ('queued', 'running', 'completed', 'recovered', 'interrupted', 'failed', 'cancelled')),
    error_message          TEXT NOT NULL DEFAULT '',
    requested_at           TIMESTAMP NOT NULL,
    started_at             TIMESTAMP,
    completed_at           TIMESTAMP,
    diff_json              TEXT NOT NULL DEFAULT '',
    rolled_back_at         TIMESTAMP,
    plan_json              TEXT NOT NULL DEFAULT '',
    branch_id              TEXT NOT NULL DEFAULT '',
    promotion_started_at   TIMESTAMP,
    promoted_to_turn_id    TEXT REFERENCES conversation_turns_next(id) ON DELETE SET NULL,
    retry_of_turn_id       TEXT REFERENCES conversation_turns_next(id) ON DELETE RESTRICT
);

INSERT INTO conversation_turns_next (
    id, conversation_id, handled_by_session_id, provider_turn_id,
    controller_generation, state, error_message, requested_at, started_at,
    completed_at, diff_json, rolled_back_at, plan_json, branch_id,
    promotion_started_at, promoted_to_turn_id, retry_of_turn_id
)
SELECT
    id, conversation_id, handled_by_session_id, provider_turn_id,
    controller_generation, state, error_message, requested_at, started_at,
    completed_at, diff_json, rolled_back_at, plan_json, branch_id,
    promotion_started_at, promoted_to_turn_id, retry_of_turn_id
FROM conversation_turns;

DROP TABLE conversation_turns;
ALTER TABLE conversation_turns_next RENAME TO conversation_turns;

CREATE INDEX idx_conversation_turns_conversation
    ON conversation_turns(conversation_id, requested_at);
CREATE UNIQUE INDEX idx_conversation_turns_provider
    ON conversation_turns(conversation_id, provider_turn_id)
    WHERE provider_turn_id <> '';
CREATE INDEX idx_conversation_turns_branch
    ON conversation_turns(branch_id, requested_at);
CREATE INDEX idx_conversation_turns_retry_source
    ON conversation_turns(conversation_id, retry_of_turn_id)
    WHERE retry_of_turn_id IS NOT NULL;

CREATE TRIGGER conversation_turns_branch_insert
AFTER INSERT ON conversation_turns
WHEN NEW.branch_id = ''
BEGIN
    UPDATE conversation_turns
    SET branch_id = (SELECT active_branch_id FROM conversations WHERE id = NEW.conversation_id)
    WHERE id = NEW.id;
END;

CREATE TRIGGER conversation_turns_cdc_update
AFTER UPDATE ON conversation_turns
WHEN OLD.state <> NEW.state
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT s.project_id, s.id, 'session_updated',
           json_object('id', s.id, 'sessionId', s.id, 'conversationId', NEW.conversation_id,
                       'activity', s.activity_state,
                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),
           COALESCE(NEW.completed_at, NEW.started_at, NEW.requested_at)
    FROM sessions s
    WHERE s.id = NEW.handled_by_session_id;
END;

PRAGMA foreign_keys=ON;
PRAGMA foreign_key_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Cancelled outcomes cannot be represented faithfully by the previous schema,
-- the same reason migration 0118 refuses to reverse.
SELECT 1;
-- +goose StatementEnd
