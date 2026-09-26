-- +goose Up
-- +goose StatementBegin
-- Remove legacy Claude Code usage rows and the Anthropic provider vocabulary,
-- then narrow the CHECK constraints to match the current domain enums.
-- The daemon no longer supports Claude Code, and no production usage parser
-- emits Anthropic-vocabulary events yet, so legacy rows are discarded rather
-- than remapped. Triggers and the three usage views are dropped before the
-- table rebuilds (they reference the renamed tables) and recreated after.

DROP VIEW usage_codex_pending_children;
DROP VIEW usage_codex_source_discovery;
DROP VIEW usage_session_integrity;
DROP TRIGGER usage_sources_cdc_update;
DROP TRIGGER usage_bindings_cdc_insert;
DROP TRIGGER usage_bindings_cdc_update;

DELETE FROM model_usage_events WHERE provider_id = 'anthropic';
DELETE FROM usage_sources WHERE kind IN ('claude_main', 'claude_subagent');
DELETE FROM usage_sources WHERE binding_id IN (SELECT id FROM usage_bindings WHERE harness = 'claude-code');
DELETE FROM usage_bindings WHERE harness = 'claude-code';

-- usage_bindings: harness loses 'claude-code'.
CREATE TABLE "usage_bindings_new" (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id         TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    harness            TEXT NOT NULL CHECK (harness IN ('codex', 'kimi', 'opencode')),
    native_root_id     TEXT NOT NULL CHECK (trim(native_root_id) <> ''),
    initial_model_id   TEXT NOT NULL DEFAULT '',
    state              TEXT NOT NULL CHECK (state IN ('discovering', 'active', 'finalizing', 'complete', 'partial')),
    last_error_code    TEXT NOT NULL DEFAULT '',
    updated_at         TIMESTAMP NOT NULL,
    provider_hint      TEXT NOT NULL DEFAULT '',
    UNIQUE (session_id, harness, native_root_id)
);
INSERT INTO "usage_bindings_new" SELECT * FROM "usage_bindings";
DROP TABLE "usage_bindings";
ALTER TABLE "usage_bindings_new" RENAME TO "usage_bindings";

CREATE INDEX idx_usage_bindings_session_state ON usage_bindings (session_id, state);

-- usage_sources: kind loses 'claude_main' and 'claude_subagent'.
CREATE TABLE "usage_sources_new" (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id          INTEGER NOT NULL REFERENCES usage_bindings (id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('codex_rollout', 'kimi_wire')),
    native_session_id   TEXT NOT NULL DEFAULT '',
    subagent_id         TEXT NOT NULL DEFAULT '',
    artifact_path       TEXT NOT NULL CHECK (trim(artifact_path) <> ''),
    file_identity       TEXT NOT NULL DEFAULT '',
    generation          INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    byte_offset         INTEGER NOT NULL DEFAULT 0 CHECK (byte_offset >= 0),
    parser_state_json   TEXT NOT NULL DEFAULT '{}',
    state               TEXT NOT NULL CHECK (state IN ('pending', 'active', 'complete', 'error')),
    failure_count       INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    anomaly_count       INTEGER NOT NULL DEFAULT 0 CHECK (anomaly_count >= 0),
    next_retry_at        TIMESTAMP,
    last_error_code     TEXT NOT NULL DEFAULT '',
    updated_at          TIMESTAMP NOT NULL,
    UNIQUE (binding_id, artifact_path, generation)
);
INSERT INTO "usage_sources_new" SELECT * FROM "usage_sources";
DROP TABLE "usage_sources";
ALTER TABLE "usage_sources_new" RENAME TO "usage_sources";

CREATE INDEX idx_usage_sources_state_retry ON usage_sources (state, next_retry_at);
CREATE INDEX idx_usage_sources_binding_kind ON usage_sources (binding_id, kind);
CREATE INDEX idx_usage_sources_codex_native_latest
    ON usage_sources (kind, native_session_id, binding_id, generation DESC, id DESC);

-- model_usage_events: provider_id loses 'anthropic'.
CREATE TABLE "model_usage_events_new" (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id                  INTEGER NOT NULL REFERENCES usage_bindings (id) ON DELETE CASCADE,
    usage_source_id             INTEGER NOT NULL REFERENCES usage_sources (id) ON DELETE CASCADE,
    provider_id                 TEXT NOT NULL CHECK (provider_id IN ('openai')),
    billing_provider_id         TEXT CHECK (billing_provider_id IS NULL OR trim(billing_provider_id) <> ''),
    model_id                    TEXT NOT NULL CHECK (trim(model_id) <> ''),
    usage_measurement_kind      TEXT NOT NULL
        CHECK (usage_measurement_kind IN ('native_reported', 'open_agents_estimated', 'mixed', 'unknown')),
    input_tokens                INTEGER CHECK (input_tokens IS NULL OR input_tokens >= 0),
    cached_input_tokens         INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    uncached_input_tokens       INTEGER CHECK (uncached_input_tokens IS NULL OR uncached_input_tokens >= 0),
    output_tokens               INTEGER CHECK (output_tokens IS NULL OR output_tokens >= 0),
    provider_usage_json         TEXT
        CHECK (provider_usage_json IS NULL
               OR (json_valid(provider_usage_json) AND json_type(provider_usage_json, '$') = 'object')),
    source_event_key            TEXT NOT NULL CHECK (trim(source_event_key) <> ''),
    created_at                  TIMESTAMP,
    input_cost_nanos            INTEGER CHECK (input_cost_nanos IS NULL OR input_cost_nanos >= 0),
    cached_input_cost_nanos     INTEGER CHECK (cached_input_cost_nanos IS NULL OR cached_input_cost_nanos >= 0),
    output_cost_nanos           INTEGER CHECK (output_cost_nanos IS NULL OR output_cost_nanos >= 0),
    estimated_cost_nanos        INTEGER CHECK (estimated_cost_nanos IS NULL OR estimated_cost_nanos >= 0),
    pricing_version             TEXT NOT NULL DEFAULT '',
    billing_provider_source     TEXT CHECK (billing_provider_source IS NULL
                                             OR billing_provider_source IN ('observed', 'inferred')),
    UNIQUE (binding_id, source_event_key),
    CHECK (input_tokens IS NULL OR cached_input_tokens IS NULL OR uncached_input_tokens IS NULL
           OR input_tokens = cached_input_tokens + uncached_input_tokens)
);
INSERT INTO "model_usage_events_new" SELECT * FROM "model_usage_events";
DROP TABLE "model_usage_events";
ALTER TABLE "model_usage_events_new" RENAME TO "model_usage_events";

CREATE INDEX idx_model_usage_events_binding_model ON model_usage_events (binding_id, model_id);
CREATE INDEX idx_model_usage_events_usage_source ON model_usage_events (usage_source_id);
CREATE INDEX idx_model_usage_events_cost_candidates
    ON model_usage_events (billing_provider_id, pricing_version, id)
    WHERE estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_canonical_cost_candidates
    ON model_usage_events (
        CASE lower(trim(billing_provider_id))
            WHEN 'z.ai' THEN 'zai'
            ELSE lower(trim(billing_provider_id))
        END,
        id
    )
    WHERE billing_provider_id IS NOT NULL
      AND estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_open_attribution
    ON model_usage_events (usage_source_id, id)
    WHERE billing_provider_id IS NULL OR billing_provider_source = 'inferred';

-- Recreate the CDC triggers dropped above. These feed change_log; losing them
-- silently stops change events for usage rows.
CREATE TRIGGER usage_sources_cdc_update AFTER UPDATE ON usage_sources
WHEN OLD.anomaly_count IS NOT NEW.anomaly_count
  OR OLD.last_error_code IS NOT NEW.last_error_code
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT s.project_id, ub.session_id, 'session_updated', json_object('id', ub.session_id), NEW.updated_at
    FROM usage_bindings ub JOIN sessions s ON s.id = ub.session_id WHERE ub.id = NEW.binding_id;
END;

CREATE TRIGGER usage_bindings_cdc_insert AFTER INSERT ON usage_bindings BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),
            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);
END;

CREATE TRIGGER usage_bindings_cdc_update AFTER UPDATE ON usage_bindings BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),
            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);
END;

CREATE VIEW usage_codex_source_discovery AS
SELECT source_id, binding_id, native_session_id,
    CASE WHEN child_ids_json IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM json_each(child_ids_json) WHERE type <> 'text'
    ) THEN child_ids_json ELSE '[]' END AS discovered_child_ids_json,
    CASE WHEN child_ids_json IS NOT NULL AND EXISTS (
        SELECT 1 FROM json_each(child_ids_json) WHERE type <> 'text'
    ) THEN 1 ELSE 0 END AS has_mixed_child_types
FROM (
    SELECT id AS source_id, binding_id, native_session_id,
        CASE WHEN json_valid(parser_state_json)
          AND json_type(parser_state_json, '$') = 'object'
          AND json_type(parser_state_json, '$.version') = 'integer'
          AND json_extract(parser_state_json, '$.version') = 1
          AND json_type(parser_state_json, '$.source_kind') = 'text'
          AND json_extract(parser_state_json, '$.source_kind') = 'codex_rollout'
          AND json_type(parser_state_json, '$.codex') = 'object'
          AND json_type(parser_state_json, '$.codex.discovered_child_ids') = 'array'
        THEN json_extract(parser_state_json, '$.codex.discovered_child_ids') END AS child_ids_json
    FROM usage_sources WHERE kind = 'codex_rollout'
);

CREATE VIEW usage_codex_pending_children AS
SELECT spawning.binding_id, CAST(discovered.value AS TEXT) AS native_session_id
FROM usage_codex_source_discovery spawning
JOIN json_each(spawning.discovered_child_ids_json) discovered
WHERE discovered.type = 'text'
  AND length(discovered.value) = 36
  AND substr(discovered.value, 9, 1) = '-'
  AND substr(discovered.value, 14, 1) = '-'
  AND substr(discovered.value, 19, 1) = '-'
  AND substr(discovered.value, 24, 1) = '-'
  AND lower(discovered.value) = discovered.value
  AND length(replace(discovered.value, '-', '')) = 32
  AND replace(discovered.value, '-', '') NOT GLOB '*[^0-9a-f]*'
  AND spawning.source_id = (
      SELECT latest.id FROM usage_sources latest
      WHERE latest.binding_id = spawning.binding_id
        AND latest.kind = 'codex_rollout'
        AND latest.native_session_id = spawning.native_session_id
      ORDER BY latest.generation DESC, latest.id DESC LIMIT 1
  )
  AND NOT EXISTS (
      SELECT 1 FROM usage_sources registered
      WHERE registered.binding_id = spawning.binding_id
        AND registered.kind = 'codex_rollout'
        AND registered.native_session_id = CAST(discovered.value AS TEXT)
  );

CREATE VIEW usage_session_integrity AS
SELECT ub.session_id,
    CAST(MAX(CASE
        WHEN ub.state = 'partial'
          OR ub.last_error_code NOT IN ('', 'source_discovery_pending', 'artifact_missing', 'source_read_failed')
          OR (us.last_error_code <> 'artifact_replaced' AND (
              us.anomaly_count > 0
              OR us.last_error_code NOT IN ('', 'source_discovery_pending', 'artifact_missing', 'source_read_failed')
          ))
        THEN 1 ELSE 0
    END) AS INTEGER) AS incomplete
FROM usage_bindings ub
LEFT JOIN usage_sources us ON us.binding_id = ub.id
GROUP BY ub.session_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Reverse the vocabulary narrowing. Legacy rows are not restored: the deleted
-- Claude/Anthropic rows cannot be reconstructed, and the widened CHECKs accept
-- them again so historical data would insert cleanly if it still existed.
-- Triggers and the three usage views are dropped before the table rebuilds
-- and recreated after.
DROP VIEW usage_codex_pending_children;
DROP VIEW usage_codex_source_discovery;
DROP VIEW usage_session_integrity;
DROP TRIGGER usage_sources_cdc_update;
DROP TRIGGER usage_bindings_cdc_insert;
DROP TRIGGER usage_bindings_cdc_update;

CREATE TABLE "usage_bindings_old" (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id         TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    harness            TEXT NOT NULL CHECK (harness IN ('claude-code', 'codex', 'kimi', 'opencode')),
    native_root_id     TEXT NOT NULL CHECK (trim(native_root_id) <> ''),
    initial_model_id   TEXT NOT NULL DEFAULT '',
    state              TEXT NOT NULL CHECK (state IN ('discovering', 'active', 'finalizing', 'complete', 'partial')),
    last_error_code    TEXT NOT NULL DEFAULT '',
    updated_at         TIMESTAMP NOT NULL,
    provider_hint      TEXT NOT NULL DEFAULT '',
    UNIQUE (session_id, harness, native_root_id)
);
INSERT INTO "usage_bindings_old" SELECT * FROM "usage_bindings";
DROP TABLE "usage_bindings";
ALTER TABLE "usage_bindings_old" RENAME TO "usage_bindings";
CREATE INDEX idx_usage_bindings_session_state ON usage_bindings (session_id, state);

CREATE TABLE "usage_sources_old" (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id          INTEGER NOT NULL REFERENCES usage_bindings (id) ON DELETE CASCADE,
    kind                TEXT NOT NULL CHECK (kind IN ('claude_main', 'claude_subagent', 'codex_rollout', 'kimi_wire')),
    native_session_id   TEXT NOT NULL DEFAULT '',
    subagent_id         TEXT NOT NULL DEFAULT '',
    artifact_path       TEXT NOT NULL CHECK (trim(artifact_path) <> ''),
    file_identity       TEXT NOT NULL DEFAULT '',
    generation          INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    byte_offset         INTEGER NOT NULL DEFAULT 0 CHECK (byte_offset >= 0),
    parser_state_json   TEXT NOT NULL DEFAULT '{}',
    state               TEXT NOT NULL CHECK (state IN ('pending', 'active', 'complete', 'error')),
    failure_count       INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    anomaly_count       INTEGER NOT NULL DEFAULT 0 CHECK (anomaly_count >= 0),
    next_retry_at        TIMESTAMP,
    last_error_code     TEXT NOT NULL DEFAULT '',
    updated_at          TIMESTAMP NOT NULL,
    UNIQUE (binding_id, artifact_path, generation)
);
INSERT INTO "usage_sources_old" SELECT * FROM "usage_sources";
DROP TABLE "usage_sources";
ALTER TABLE "usage_sources_old" RENAME TO "usage_sources";
CREATE INDEX idx_usage_sources_state_retry ON usage_sources (state, next_retry_at);
CREATE INDEX idx_usage_sources_binding_kind ON usage_sources (binding_id, kind);
CREATE INDEX idx_usage_sources_codex_native_latest
    ON usage_sources (kind, native_session_id, binding_id, generation DESC, id DESC);

CREATE TABLE "model_usage_events_old" (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id                  INTEGER NOT NULL REFERENCES usage_bindings (id) ON DELETE CASCADE,
    usage_source_id             INTEGER NOT NULL REFERENCES usage_sources (id) ON DELETE CASCADE,
    provider_id                 TEXT NOT NULL CHECK (provider_id IN ('openai', 'anthropic')),
    billing_provider_id         TEXT CHECK (billing_provider_id IS NULL OR trim(billing_provider_id) <> ''),
    model_id                    TEXT NOT NULL CHECK (trim(model_id) <> ''),
    usage_measurement_kind      TEXT NOT NULL
        CHECK (usage_measurement_kind IN ('native_reported', 'open_agents_estimated', 'mixed', 'unknown')),
    input_tokens                INTEGER CHECK (input_tokens IS NULL OR input_tokens >= 0),
    cached_input_tokens         INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    uncached_input_tokens       INTEGER CHECK (uncached_input_tokens IS NULL OR uncached_input_tokens >= 0),
    output_tokens               INTEGER CHECK (output_tokens IS NULL OR output_tokens >= 0),
    provider_usage_json         TEXT
        CHECK (provider_usage_json IS NULL
               OR (json_valid(provider_usage_json) AND json_type(provider_usage_json, '$') = 'object')),
    source_event_key            TEXT NOT NULL CHECK (trim(source_event_key) <> ''),
    created_at                  TIMESTAMP,
    input_cost_nanos            INTEGER CHECK (input_cost_nanos IS NULL OR input_cost_nanos >= 0),
    cached_input_cost_nanos     INTEGER CHECK (cached_input_cost_nanos IS NULL OR cached_input_cost_nanos >= 0),
    output_cost_nanos           INTEGER CHECK (output_cost_nanos IS NULL OR output_cost_nanos >= 0),
    estimated_cost_nanos        INTEGER CHECK (estimated_cost_nanos IS NULL OR estimated_cost_nanos >= 0),
    pricing_version             TEXT NOT NULL DEFAULT '',
    billing_provider_source     TEXT CHECK (billing_provider_source IS NULL
                                             OR billing_provider_source IN ('observed', 'inferred')),
    UNIQUE (binding_id, source_event_key),
    CHECK (input_tokens IS NULL OR cached_input_tokens IS NULL OR uncached_input_tokens IS NULL
           OR input_tokens = cached_input_tokens + uncached_input_tokens)
);
INSERT INTO "model_usage_events_old" SELECT * FROM "model_usage_events";
DROP TABLE "model_usage_events";
ALTER TABLE "model_usage_events_old" RENAME TO "model_usage_events";
CREATE INDEX idx_model_usage_events_binding_model ON model_usage_events (binding_id, model_id);
CREATE INDEX idx_model_usage_events_usage_source ON model_usage_events (usage_source_id);
CREATE INDEX idx_model_usage_events_cost_candidates
    ON model_usage_events (billing_provider_id, pricing_version, id)
    WHERE estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_canonical_cost_candidates
    ON model_usage_events (
        CASE lower(trim(billing_provider_id))
            WHEN 'z.ai' THEN 'zai'
            ELSE lower(trim(billing_provider_id))
        END,
        id
    )
    WHERE billing_provider_id IS NOT NULL
      AND estimated_cost_nanos IS NULL;
CREATE INDEX idx_model_usage_events_open_attribution
    ON model_usage_events (usage_source_id, id)
    WHERE billing_provider_id IS NULL OR billing_provider_source = 'inferred';

CREATE TRIGGER usage_sources_cdc_update AFTER UPDATE ON usage_sources
WHEN OLD.anomaly_count IS NOT NEW.anomaly_count
  OR OLD.last_error_code IS NOT NEW.last_error_code
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT s.project_id, ub.session_id, 'session_updated', json_object('id', ub.session_id), NEW.updated_at
    FROM usage_bindings ub JOIN sessions s ON s.id = ub.session_id WHERE ub.id = NEW.binding_id;
END;

CREATE TRIGGER usage_bindings_cdc_insert AFTER INSERT ON usage_bindings BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),
            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);
END;

CREATE TRIGGER usage_bindings_cdc_update AFTER UPDATE ON usage_bindings BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),
            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);
END;

CREATE VIEW usage_codex_source_discovery AS
SELECT source_id, binding_id, native_session_id,
    CASE WHEN child_ids_json IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM json_each(child_ids_json) WHERE type <> 'text'
    ) THEN child_ids_json ELSE '[]' END AS discovered_child_ids_json,
    CASE WHEN child_ids_json IS NOT NULL AND EXISTS (
        SELECT 1 FROM json_each(child_ids_json) WHERE type <> 'text'
    ) THEN 1 ELSE 0 END AS has_mixed_child_types
FROM (
    SELECT id AS source_id, binding_id, native_session_id,
        CASE WHEN json_valid(parser_state_json)
          AND json_type(parser_state_json, '$') = 'object'
          AND json_type(parser_state_json, '$.version') = 'integer'
          AND json_extract(parser_state_json, '$.version') = 1
          AND json_type(parser_state_json, '$.source_kind') = 'text'
          AND json_extract(parser_state_json, '$.source_kind') = 'codex_rollout'
          AND json_type(parser_state_json, '$.codex') = 'object'
          AND json_type(parser_state_json, '$.codex.discovered_child_ids') = 'array'
        THEN json_extract(parser_state_json, '$.codex.discovered_child_ids') END AS child_ids_json
    FROM usage_sources WHERE kind = 'codex_rollout'
);

CREATE VIEW usage_codex_pending_children AS
SELECT spawning.binding_id, CAST(discovered.value AS TEXT) AS native_session_id
FROM usage_codex_source_discovery spawning
JOIN json_each(spawning.discovered_child_ids_json) discovered
WHERE discovered.type = 'text'
  AND length(discovered.value) = 36
  AND substr(discovered.value, 9, 1) = '-'
  AND substr(discovered.value, 14, 1) = '-'
  AND substr(discovered.value, 19, 1) = '-'
  AND substr(discovered.value, 24, 1) = '-'
  AND lower(discovered.value) = discovered.value
  AND length(replace(discovered.value, '-', '')) = 32
  AND replace(discovered.value, '-', '') NOT GLOB '*[^0-9a-f]*'
  AND spawning.source_id = (
      SELECT latest.id FROM usage_sources latest
      WHERE latest.binding_id = spawning.binding_id
        AND latest.kind = 'codex_rollout'
        AND latest.native_session_id = spawning.native_session_id
      ORDER BY latest.generation DESC, latest.id DESC LIMIT 1
  )
  AND NOT EXISTS (
      SELECT 1 FROM usage_sources registered
      WHERE registered.binding_id = spawning.binding_id
        AND registered.kind = 'codex_rollout'
        AND registered.native_session_id = CAST(discovered.value AS TEXT)
  );

CREATE VIEW usage_session_integrity AS
SELECT ub.session_id,
    CAST(MAX(CASE
        WHEN ub.state = 'partial'
          OR ub.last_error_code NOT IN ('', 'source_discovery_pending', 'artifact_missing', 'source_read_failed')
          OR (us.last_error_code <> 'artifact_replaced' AND (
              us.anomaly_count > 0
              OR us.last_error_code NOT IN ('', 'source_discovery_pending', 'artifact_missing', 'source_read_failed')
          ))
        THEN 1 ELSE 0
    END) AS INTEGER) AS incomplete
FROM usage_bindings ub
LEFT JOIN usage_sources us ON us.binding_id = ub.id
GROUP BY ub.session_id;
-- +goose StatementEnd
