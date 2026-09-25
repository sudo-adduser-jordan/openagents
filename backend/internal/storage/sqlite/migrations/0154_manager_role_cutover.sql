-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin

-- The role is now called manager in the active product vocabulary. Keep the
-- existing physical sessions schema (including any harness-check variant that
-- a user's database already carries) and change only its role constraint.
-- Rebuilding the whole table here would accidentally narrow legacy harness
-- checks such as QM and Muse during migration.
PRAGMA writable_schema=ON;

UPDATE sqlite_schema
SET sql = replace(
    replace(
        replace(
            sql,
            'CHECK (kind IN (' || char(39) || 'worker' || char(39) || ', ' || char(39) || 'orchestrator' || char(39) || '))',
            'CHECK (kind IN (' || char(39) || 'worker' || char(39) || ', ' || char(39) || 'manager' || char(39) || '))'
        ),
        'CHECK (kind IN (' || char(39) || 'worker' || char(39) || ',' || char(39) || 'orchestrator' || char(39) || '))',
        'CHECK (kind IN (' || char(39) || 'worker' || char(39) || ',' || char(39) || 'manager' || char(39) || '))'
    ),
    'UNIQUE (project_id, num)',
    'CHECK ((kind = ' || char(39) || 'manager' || char(39) || ' AND workflow_mode IN (' || char(39) || 'planning' || char(39) || ', ' || char(39) || 'manager' || char(39) || ')) OR (kind = ' || char(39) || 'worker' || char(39) || ' AND workflow_mode IN (' || char(39) || 'planning' || char(39) || ', ' || char(39) || 'building' || char(39) || '))), UNIQUE (project_id, num)'
)
WHERE type = 'table' AND name = 'sessions';

PRAGMA writable_schema=RESET;

BEGIN IMMEDIATE;

-- Do not emit migration-time CDC events or advance session revisions while
-- translating the role and its old build/planning posture. Recreate the exact
-- current triggers after the data update.
DROP TRIGGER IF EXISTS sessions_cdc_update;
DROP TRIGGER IF EXISTS sessions_revision_update;

-- Old build-mode coordinators become Manager mode. Existing planning-mode
-- coordinators remain planning-only, preserving their existing delegation
-- gate. Any unknown legacy value is normalized to the safe role default.
UPDATE sessions
SET kind = 'manager',
    workflow_mode = CASE
        WHEN workflow_mode = 'building' THEN 'manager'
        WHEN workflow_mode = 'planning' THEN 'planning'
        ELSE 'manager'
    END
WHERE kind = 'orchestrator';

-- Project configuration is a typed JSON blob. Rename only the two role-owned
-- keys while preserving every unrelated setting and nested override object.
UPDATE projects
SET config = json_set(
    json_remove(config, '$.orchestrator'),
    '$.manager', json_extract(config, '$.orchestrator')
)
WHERE config IS NOT NULL
  AND json_valid(config)
  AND json_type(config, '$.orchestrator') IS NOT NULL;

UPDATE projects
SET config = json_set(
    json_remove(config, '$.orchestratorRules'),
    '$.managerRules', json_extract(config, '$.orchestratorRules')
)
WHERE config IS NOT NULL
  AND json_valid(config)
  AND json_type(config, '$.orchestratorRules') IS NOT NULL;

CREATE TRIGGER sessions_cdc_update
AFTER UPDATE ON sessions
WHEN OLD.activity_state <> NEW.activity_state
    OR OLD.is_terminated <> NEW.is_terminated
    OR (OLD.first_signal_at IS NULL AND NEW.first_signal_at IS NOT NULL)
    OR OLD.preview_url <> NEW.preview_url
    OR OLD.preview_revision <> NEW.preview_revision
    OR OLD.display_name <> NEW.display_name
    OR OLD.terminate_on_pr_merge <> NEW.terminate_on_pr_merge
    OR OLD.is_pinned <> NEW.is_pinned
    OR OLD.pinned_at <> NEW.pinned_at
    OR (OLD.pinned_at IS NULL AND NEW.pinned_at IS NOT NULL)
    OR (OLD.pinned_at IS NOT NULL AND NEW.pinned_at IS NULL)
    OR OLD.session_mode <> NEW.session_mode
    OR OLD.auto_inject_review <> NEW.auto_inject_review
    OR OLD.auto_review_enabled <> NEW.auto_review_enabled
    OR OLD.harness <> NEW.harness
    OR OLD.runtime_launch_id <> NEW.runtime_launch_id
    OR OLD.agent_session_id <> NEW.agent_session_id
    OR OLD.native_transcript_path <> NEW.native_transcript_path
    OR OLD.auto_inject_ci <> NEW.auto_inject_ci
    OR OLD.latest_user_prompt_at IS NOT NEW.latest_user_prompt_at
    OR OLD.workflow_mode <> NEW.workflow_mode
    OR OLD.review_locked <> NEW.review_locked
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NEW.id, 'session_updated',
        json_object(
            'id', NEW.id,
            'activity', NEW.activity_state,
            'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END),
            'terminateOnPrMerge', json(CASE WHEN NEW.terminate_on_pr_merge THEN 'true' ELSE 'false' END),
            'previewUrl', NEW.preview_url,
            'previewRevision', NEW.preview_revision,
            'isPinned', json(CASE WHEN NEW.is_pinned THEN 'true' ELSE 'false' END),
            'mode', NEW.session_mode,
            'autoInjectReview', json(CASE WHEN NEW.auto_inject_review THEN 'true' ELSE 'false' END),
            'autoInjectCI', json(CASE WHEN NEW.auto_inject_ci THEN 'true' ELSE 'false' END),
            'autoReviewEnabled', json(CASE WHEN NEW.auto_review_enabled THEN 'true' ELSE 'false' END),
            'workflowMode', NEW.workflow_mode,
            'reviewLocked', json(CASE WHEN NEW.review_locked THEN 'true' ELSE 'false' END)
        ),
        NEW.updated_at);
END;

CREATE TRIGGER sessions_revision_update
AFTER UPDATE ON sessions
WHEN NEW.revision = OLD.revision
BEGIN
    UPDATE sessions SET revision = OLD.revision + 1 WHERE id = NEW.id;
END;

PRAGMA foreign_key_check;
COMMIT;

-- +goose StatementEnd

-- +goose Down
-- Additive identity cutovers do not restore a retired role vocabulary.
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
