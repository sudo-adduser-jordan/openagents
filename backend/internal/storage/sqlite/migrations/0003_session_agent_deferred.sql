-- +goose Up
-- +goose StatementBegin

-- Durable "staged, not yet started" fact for a session created without its agent.
--
-- `open-agents spawn --no-start` creates the whole session -- seed row, worktree,
-- provisioning, attachments, resolved prompt -- but launches no agent process and
-- no chat controller. The board has to tell that apart from a session that ran
-- and was later stopped, and the difference must survive daemon restarts, so it
-- cannot be inferred from the absence of a runtime handle.
--
-- It is a column rather than a new `activity_state` value on purpose. SQLite
-- cannot ALTER a CHECK constraint, so admitting a value there would mean
-- rebuilding the whole ~60-column sessions table together with its change_log
-- triggers. Reusing `idle` was the other option and it reads wrong: a staged
-- session reports no hooks, so `silentPastGrace` would demote it to `no_signal`
-- after the grace period, which the contract defines as a broken hook pipeline.
--
-- DEFAULT 0 is load-bearing: every row written before this migration had its
-- agent launched, and every synthetic `SessionRecord` in the codebase has Go's
-- zero value, so existing behaviour is preserved with no backfill.
ALTER TABLE sessions ADD COLUMN agent_deferred BOOLEAN NOT NULL DEFAULT 0;

-- Recreated so the staged -> started transition announces itself. Without the
-- `agent_deferred` condition, a Chat session staged and then started would write
-- no listed column (its activity stays `idle`; its provider conversation id is
-- not part of the trigger) and a connected board would keep rendering the card
-- as "Not started" until some unrelated change arrived. The terminal path already
-- tripped `runtime_launch_id`, but relying on that would make Chat the odd one
-- out. Payload-only addition; every other condition is carried over verbatim.
DROP TRIGGER sessions_cdc_update;

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
    OR OLD.agent_deferred <> NEW.agent_deferred
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
            'reviewLocked', json(CASE WHEN NEW.review_locked THEN 'true' ELSE 'false' END),
            'agentDeferred', json(CASE WHEN NEW.agent_deferred THEN 'true' ELSE 'false' END)
        ),
        NEW.updated_at);
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS sessions_cdc_update;

-- Restores the 0001 trigger exactly: no `agent_deferred` condition, no
-- `agentDeferred` payload key.
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

-- SQLite's DROP COLUMN arrived in 3.35; the daemon pins modernc.org/sqlite
-- v1.51.0, well past it. No other table references this column, so the drop
-- needs no table rebuild and no index changes.
ALTER TABLE sessions DROP COLUMN agent_deferred;
-- +goose StatementEnd
