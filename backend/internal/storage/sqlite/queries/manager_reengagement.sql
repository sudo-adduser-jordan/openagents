-- name: ScheduleManagerReengagement :exec
INSERT INTO manager_reengagements (
    session_id, attempt_count, next_attempt_at, progress_since_attempt,
    attention_notified, state, created_at, updated_at
) VALUES (?, 0, ?, 0, 0, 'active', ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    attempt_count = CASE
        WHEN manager_reengagements.progress_since_attempt THEN 0
        ELSE manager_reengagements.attempt_count
    END,
    next_attempt_at = CASE
        WHEN manager_reengagements.progress_since_attempt THEN excluded.next_attempt_at
        ELSE manager_reengagements.next_attempt_at
    END,
    progress_since_attempt = 0,
    attention_notified = CASE
        WHEN manager_reengagements.progress_since_attempt THEN 0
        ELSE manager_reengagements.attention_notified
    END,
    state = CASE
        WHEN manager_reengagements.state = 'exhausted'
            AND manager_reengagements.progress_since_attempt THEN 'active'
        ELSE manager_reengagements.state
    END,
    updated_at = excluded.updated_at
WHERE manager_reengagements.state <> 'completed';

-- name: EnsureManagerReengagement :exec
INSERT INTO manager_reengagements (
    session_id, attempt_count, next_attempt_at, progress_since_attempt,
    attention_notified, state, created_at, updated_at
) VALUES (?, 0, ?, 0, 0, 'active', ?, ?)
ON CONFLICT(session_id) DO NOTHING;

-- name: MarkManagerReengagementProgress :execrows
UPDATE manager_reengagements SET
    progress_since_attempt = 1,
    updated_at = ?
WHERE session_id = ? AND state <> 'completed';

-- name: ListDueManagerReengagements :many
SELECT session_id, attempt_count, next_attempt_at, last_attempt_at,
    progress_since_attempt, attention_notified, state, created_at, updated_at
FROM manager_reengagements
WHERE state = 'active' AND next_attempt_at <= ?
ORDER BY next_attempt_at, session_id;

-- name: GetManagerReengagement :one
SELECT session_id, attempt_count, next_attempt_at, last_attempt_at,
    progress_since_attempt, attention_notified, state, created_at, updated_at
FROM manager_reengagements
WHERE session_id = ?;

-- name: RecordManagerReengagementAttempt :one
UPDATE manager_reengagements SET
    attempt_count = attempt_count + 1,
    next_attempt_at = ?,
    last_attempt_at = ?,
    progress_since_attempt = 0,
    state = CASE WHEN attempt_count + 1 >= ? THEN 'exhausted' ELSE 'active' END,
    updated_at = ?
WHERE session_id = ? AND state = 'active'
RETURNING session_id, attempt_count, next_attempt_at, last_attempt_at,
    progress_since_attempt, attention_notified, state, created_at, updated_at;

-- name: ListPendingManagerAttention :many
SELECT session_id, attempt_count, next_attempt_at, last_attempt_at,
    progress_since_attempt, attention_notified, state, created_at, updated_at
FROM manager_reengagements
WHERE state = 'exhausted' AND attention_notified = 0
ORDER BY updated_at, session_id;

-- name: MarkManagerAttentionNotified :execrows
UPDATE manager_reengagements SET
    attention_notified = 1,
    updated_at = ?
WHERE session_id = ? AND state = 'exhausted' AND attention_notified = 0;

-- name: CompleteManagerReengagement :execrows
INSERT INTO manager_reengagements (
    session_id, attempt_count, next_attempt_at, progress_since_attempt,
    attention_notified, state, created_at, updated_at
) VALUES (?, 0, ?, 0, 1, 'completed', ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    progress_since_attempt = 0,
    attention_notified = 1,
    state = 'completed',
    updated_at = excluded.updated_at;
