-- name: UpsertUsageBinding :one
INSERT INTO usage_bindings (
    session_id, harness, native_root_id, initial_model_id, state,
    last_error_code, updated_at, provider_hint
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, harness, native_root_id) DO UPDATE SET
    initial_model_id = CASE
        WHEN excluded.initial_model_id <> '' THEN excluded.initial_model_id
        ELSE usage_bindings.initial_model_id
    END,
    provider_hint = CASE
        WHEN excluded.provider_hint <> '' THEN excluded.provider_hint
        ELSE usage_bindings.provider_hint
    END,
    state = CASE
        WHEN usage_bindings.state IN ('finalizing', 'complete', 'partial')
          AND excluded.state IN ('discovering', 'active')
        THEN usage_bindings.state
        ELSE excluded.state
    END,
    last_error_code = CASE
        WHEN usage_bindings.state IN ('finalizing', 'complete', 'partial')
          AND excluded.state IN ('discovering', 'active')
        THEN usage_bindings.last_error_code
        ELSE excluded.last_error_code
    END,
    updated_at = excluded.updated_at
RETURNING *;

-- name: GetUsageBindingBySessionHarnessRoot :one
SELECT *
FROM usage_bindings
WHERE session_id = ? AND harness = ? AND native_root_id = ?;

-- name: ListUsageBindingsForSession :many
SELECT *
FROM usage_bindings
WHERE session_id = ?
ORDER BY updated_at, id;

-- name: FinalizeUsageBindingsForSessionLaunch :many
UPDATE usage_bindings
SET state = 'finalizing',
    last_error_code = '',
    updated_at = sqlc.arg(finalized_at)
WHERE usage_bindings.session_id = sqlc.arg(session_id)
  AND EXISTS (
      SELECT 1
      FROM sessions
      WHERE sessions.id = usage_bindings.session_id
        AND sessions.runtime_launch_id = sqlc.arg(expected_runtime_launch_id)
        AND sessions.revision = sqlc.arg(expected_session_revision)
        AND sessions.is_terminated = 0
  )
RETURNING *;

-- name: InsertUsageSource :one
INSERT INTO usage_sources (
    binding_id, kind, native_session_id, subagent_id, artifact_path,
    file_identity, generation, byte_offset, parser_state_json,
    state, failure_count, anomaly_count, next_retry_at, last_error_code,
    updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (binding_id, artifact_path, generation) DO UPDATE SET
    native_session_id = CASE
        WHEN excluded.native_session_id <> '' THEN excluded.native_session_id
        ELSE usage_sources.native_session_id
    END,
    subagent_id = CASE
        WHEN excluded.subagent_id <> '' THEN excluded.subagent_id
        ELSE usage_sources.subagent_id
    END,
    updated_at = excluded.updated_at
RETURNING *;

-- name: ListUsageSourcesForBinding :many
SELECT *
FROM usage_sources
WHERE binding_id = ?
ORDER BY generation, id;

-- name: ListWatchableUsageSources :many
SELECT us.*
FROM usage_sources us
JOIN usage_bindings ub ON ub.id = us.binding_id
JOIN sessions s ON s.id = ub.session_id
WHERE (s.is_terminated = 0 OR ub.state = 'finalizing')
  AND NOT (
      us.state = 'complete'
      AND us.last_error_code = 'artifact_replaced'
  )
  AND us.id = (
      SELECT latest.id
      FROM usage_sources latest
      WHERE latest.binding_id = us.binding_id
        AND latest.artifact_path = us.artifact_path
      ORDER BY latest.generation DESC, latest.id DESC
      LIMIT 1
  )
ORDER BY us.artifact_path, us.generation, us.id;

-- name: HasPendingUsageDiscovery :one
SELECT CAST(EXISTS (
    SELECT 1
    FROM usage_bindings ub
    JOIN sessions s ON s.id = ub.session_id
    WHERE (s.is_terminated = 0 OR ub.state = 'finalizing')
      AND (
          ub.state = 'discovering'
          OR ub.last_error_code = 'source_discovery_pending'
          OR EXISTS (
              SELECT 1
              FROM usage_sources source
              WHERE source.binding_id = ub.id
                AND source.state = 'error'
                AND source.last_error_code IN ('artifact_missing', 'source_read_failed')
                AND source.id = (
                    SELECT latest.id
                    FROM usage_sources latest
                    WHERE latest.binding_id = source.binding_id
                      AND latest.artifact_path = source.artifact_path
                    ORDER BY latest.generation DESC, latest.id DESC
                    LIMIT 1
                )
          )
      )
) AS INTEGER);

-- name: ListUsageDiscoveryBindings :many
SELECT ub.*
FROM usage_bindings ub
JOIN sessions s ON s.id = ub.session_id
WHERE (s.is_terminated = 0 OR ub.state = 'finalizing')
  AND ub.state = 'discovering'
ORDER BY ub.updated_at, ub.id
LIMIT ?;

-- name: GetUsageSourceWithBindingAndSession :one
SELECT
    us.id AS source_id,
    us.binding_id,
    us.kind,
    us.native_session_id,
    us.subagent_id,
    us.artifact_path,
    us.file_identity,
    us.generation,
    us.byte_offset,
    us.parser_state_json,
    us.state AS source_state,
    us.failure_count,
    us.anomaly_count,
    us.next_retry_at,
    us.last_error_code AS source_last_error_code,
    us.updated_at AS source_updated_at,
    ub.session_id,
    ub.harness,
    ub.native_root_id,
    ub.initial_model_id,
    ub.provider_hint,
    ub.state AS binding_state
FROM usage_sources us
JOIN usage_bindings ub ON ub.id = us.binding_id
WHERE us.id = ?;

-- name: UpdateUsageSourceCursor :exec
UPDATE usage_sources SET
    byte_offset = ?,
    parser_state_json = ?,
    state = ?,
    failure_count = ?,
    anomaly_count = ?,
    next_retry_at = ?,
    last_error_code = ?,
    updated_at = ?
WHERE id = ?;

-- name: UpdateUsageSourceLifecycle :execrows
UPDATE usage_sources SET
    state = sqlc.arg(state),
    failure_count = COALESCE(sqlc.narg(failure_count), failure_count),
    last_error_code = sqlc.arg(last_error_code),
    next_retry_at = sqlc.narg(next_retry_at),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: UpdateUsageBinding :execrows
UPDATE usage_bindings SET
    state = CASE
        WHEN sqlc.arg(state) = '' THEN usage_bindings.state
        ELSE sqlc.arg(state)
    END,
    last_error_code = sqlc.arg(last_error_code),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: CompleteUsageBindingIfSettled :execrows
UPDATE usage_bindings
SET state = CASE
        WHEN EXISTS (
            SELECT 1
            FROM usage_sources
            WHERE usage_sources.binding_id = sqlc.arg(usage_binding_id)
              AND last_error_code <> 'artifact_replaced'
              AND (anomaly_count > 0 OR last_error_code <> '')
        ) THEN 'partial'
        ELSE 'complete'
    END,
    last_error_code = '',
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(usage_binding_id)
  AND state = 'finalizing'
  AND EXISTS (
      SELECT 1
      FROM usage_sources
      WHERE usage_sources.binding_id = sqlc.arg(usage_binding_id)
  )
  AND NOT EXISTS (
      SELECT 1
      FROM usage_sources
      WHERE usage_sources.binding_id = sqlc.arg(usage_binding_id)
        AND state <> 'complete'
  );

-- name: GetModelUsageEventByKey :one
SELECT
    event.id, event.usage_source_id, event.provider_id, event.billing_provider_id,
    event.billing_provider_source, event.model_id, event.usage_measurement_kind,
    event.input_tokens, event.cached_input_tokens,
    event.uncached_input_tokens, event.output_tokens,
    event.provider_usage_json, event.created_at
FROM model_usage_events event
WHERE event.binding_id = ? AND event.source_event_key = ?;

-- name: InsertModelUsageEvent :one
-- The cost columns exist in the table but are never written: the pricing
-- catalog that used to fill them is gone, and nothing recomputes them.
INSERT INTO model_usage_events (
    binding_id, usage_source_id, provider_id, billing_provider_id,
    billing_provider_source, model_id, usage_measurement_kind,
    input_tokens, cached_input_tokens, uncached_input_tokens, output_tokens,
    provider_usage_json,
    source_event_key, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: RehomeOpenUsageEventToReplacementSource :execrows
-- A physically replaced transcript re-emits the same logical event under the
-- same stable key, so the replay deduplicates against the row the retired
-- generation left behind and the row keeps pointing at that generation. Repair
-- skips a source retired as artifact_replaced, and the replacement owns no row
-- for the event, so an attribution that never landed can never land.
--
-- Only an open attribution moves, and open means the same thing here as
-- everywhere else: unattributed, or attributed by inference and therefore still
-- replaceable. Leaving an inferred row on the retired generation would strand a
-- guess exactly where no observation can reach it. A row attributed by
-- observation stays put: it was collected under the generation it names, and
-- that is a fact about how it was observed rather than a stale pointer.
UPDATE model_usage_events
SET usage_source_id = sqlc.arg(usage_source_id)
WHERE model_usage_events.id = sqlc.arg(id)
  AND model_usage_events.usage_source_id = sqlc.arg(expected_usage_source_id)
  AND (model_usage_events.billing_provider_id IS NULL
       OR model_usage_events.billing_provider_source = 'inferred')
  AND EXISTS (
      SELECT 1
      FROM usage_sources replacement
      WHERE replacement.id = sqlc.arg(usage_source_id)
        AND replacement.binding_id = model_usage_events.binding_id
  );

-- name: PromoteInferredUsageEventToObserved :execrows
-- A later observation supersedes an inferred billing provider. ApplyUsageChunk
-- rehomes replacement-generation rows before this statement, so the source
-- guard also prevents promotion on a stale generation.
UPDATE model_usage_events
SET billing_provider_id = sqlc.arg(billing_provider_id),
    billing_provider_source = 'observed'
WHERE id = sqlc.arg(id)
  AND usage_source_id = sqlc.arg(expected_usage_source_id)
  AND billing_provider_id = sqlc.arg(expected_billing_provider_id)
  AND billing_provider_source = 'inferred';

-- name: HasOpenUsageAttribution :one
-- Whether this source still owns an event a repair pass could finish. Cheaper
-- than listing them, and it is asked once per applied chunk on a routed
-- binding.
SELECT CAST(EXISTS (
    SELECT 1
    FROM model_usage_events
    WHERE model_usage_events.usage_source_id = ?
      AND (model_usage_events.billing_provider_id IS NULL
           OR model_usage_events.billing_provider_source = 'inferred')
) AS INTEGER);

-- name: EnrichModelUsageEventProviderUsage :execrows
-- Replaying a durable prefix can supply the bounded provider object for an event
-- stored before the capture existed. A captured object is never overwritten.
UPDATE model_usage_events
SET provider_usage_json = sqlc.arg(provider_usage_json)
WHERE id = sqlc.arg(id)
  AND provider_usage_json IS NULL;

-- name: TouchUsageBinding :exec
UPDATE usage_bindings SET updated_at = ? WHERE id = ?;

-- name: AggregateUsageBySessionHarnessModel :many
SELECT
    ub.harness,
    mue.model_id,
    CAST(COUNT(*) AS INTEGER) AS event_count,
    CAST(COALESCE(SUM(mue.input_tokens), 0) AS INTEGER) AS input_tokens,
    CAST(COUNT(mue.input_tokens) AS INTEGER) AS known_input_token_count,
    CAST(COALESCE(SUM(mue.cached_input_tokens), 0) AS INTEGER) AS cached_input_tokens,
    CAST(COUNT(mue.cached_input_tokens) AS INTEGER) AS known_cached_input_token_count,
    CAST(COALESCE(SUM(mue.uncached_input_tokens), 0) AS INTEGER) AS uncached_input_tokens,
    CAST(COUNT(mue.uncached_input_tokens) AS INTEGER) AS known_uncached_input_token_count,
    CAST(COALESCE(SUM(mue.output_tokens), 0) AS INTEGER) AS output_tokens,
    CAST(COUNT(mue.output_tokens) AS INTEGER) AS known_output_token_count
FROM model_usage_events mue
JOIN usage_bindings ub ON ub.id = mue.binding_id
WHERE ub.session_id = ?
-- Grouped by model alone. The billing provider is not a product distinction:
-- one model stays one row even when more than one provider served it.
GROUP BY ub.harness, mue.model_id
ORDER BY SUM(mue.input_tokens + mue.output_tokens) DESC, ub.harness, mue.model_id;

-- name: GetUsageSessionIncomplete :one
SELECT CAST(COALESCE((
    SELECT incomplete FROM usage_session_integrity WHERE session_id = ?
), 0) AS INTEGER);

-- name: ListCompactSessionUsage :many
SELECT
    ub.session_id,
    CAST(COALESCE(SUM(mue.input_tokens) + SUM(mue.output_tokens), 0) AS INTEGER) AS processed_tokens,
    CAST(COUNT(mue.input_tokens) = COUNT(*) AND COUNT(mue.output_tokens) = COUNT(*) AS INTEGER) AS processed_tokens_known,
    CAST(COALESCE(integrity.incomplete, 0) AS INTEGER) AS incomplete,
    CAST(COUNT(*) AS INTEGER) AS event_count
FROM model_usage_events mue
JOIN usage_bindings ub ON ub.id = mue.binding_id
JOIN sessions s ON s.id = ub.session_id
LEFT JOIN usage_session_integrity integrity ON integrity.session_id = ub.session_id
WHERE (sqlc.arg(project_id) = '' OR s.project_id = sqlc.arg(project_id))
GROUP BY ub.session_id, s.project_id, s.num, integrity.incomplete
ORDER BY s.project_id, s.num;
