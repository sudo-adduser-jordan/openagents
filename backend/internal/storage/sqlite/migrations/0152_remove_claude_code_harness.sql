-- Claude Code is no longer a supported harness, so no new session can
-- legitimately carry harness 'claude-code'. Narrow the sessions.harness CHECK
-- by removing the 'claude-code' entry from the constraint text in every
-- historical variant.
-- SQLite cannot drop a CHECK constraint in place, so use the same
-- writable_schema sqlite_master replace pattern as 0083. Every sessions
-- constraint variant starts with ('', 'claude-code', …, so a single replace
-- of 'claude-code', covers all of them. Existing rows are not re-validated by
-- the schema text change; they simply read as history.

-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
PRAGMA writable_schema = ON;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE sqlite_master
SET sql = replace(sql,
    char(39) || 'claude-code' || char(39) || ', ',
    ''
)
WHERE type = 'table' AND name = 'sessions';
-- +goose StatementEnd
-- +goose StatementBegin
PRAGMA writable_schema = RESET;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
PRAGMA writable_schema = ON;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE sqlite_master
SET sql = replace(sql,
    '(' || char(39) || char(39) || ', ',
    '(' || char(39) || char(39) || ', ' || char(39) || 'claude-code' || char(39) || ', '
)
WHERE type = 'table' AND name = 'sessions';
-- +goose StatementEnd
-- +goose StatementBegin
PRAGMA writable_schema = RESET;
-- +goose StatementEnd